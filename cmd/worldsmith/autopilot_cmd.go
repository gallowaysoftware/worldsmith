package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// autopilotCommand is the "infinite engine you curate": when the GPU is idle, it
// deepens the named worlds and STAGES the results for the user's review — never
// publishes, never touches a world you didn't name. Designed to be run from cron
// (the loop is the cron schedule); a single invocation does one bounded pass.
//
// Safety by construction:
//   - Opt-in only: it operates solely on the slugs you pass, so a world you care
//     about (or haven't listed) is never swept in.
//   - Non-destructive: it runs `expand`, which only STAGES dossiers to .expand/
//     for `expand review` — nothing merges into a world without your accept.
//   - Capped: skips a world that already has >= --max-staged items awaiting
//     review, so proposals don't pile up unread.
//   - Idle-gated: bails if the GPU is in use, so it never fights you for it.
func autopilotCommand() *cobra.Command {
	var maxStaged, threads, idleMiB int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "autopilot <slug...>",
		Short: "When the GPU is idle, expand the named worlds and stage proposals for review (cron-friendly).",
		Long: `autopilot is the autonomous-but-curated loop. Run it (e.g. from cron) over the
worlds you want deepened. For each, if the GPU is idle and the world isn't already
holding a backlog of unreviewed proposals, it runs an expansion pass and stages the
dossiers for 'worldsmith expand review'. It only touches worlds you name, only
stages (never publishes), and bails if the GPU is busy.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutopilot(cmd, args, maxStaged, threads, idleMiB, dryRun)
		},
	}
	cmd.Flags().IntVar(&maxStaged, "max-staged", 6, "Skip a world already holding >= this many unreviewed staged dossiers.")
	cmd.Flags().IntVar(&threads, "threads", 2, "How many threads to expand per world per pass.")
	cmd.Flags().IntVar(&idleMiB, "idle-mib", 6000, "Treat the GPU as busy if more than this many MiB are in use.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report what it would do; run nothing.")
	return cmd
}

func runAutopilot(cmd *cobra.Command, slugs []string, maxStaged, threads, idleMiB int, dryRun bool) error {
	out := cmd.OutOrStdout()
	// Availability: if the long_form LLM is already up, that's autopilot's own
	// tool — proceed. Otherwise the GPU is "busy" only if something else is using
	// it past the threshold (so we don't fight the user for it). A loaded LLM no
	// longer falsely reads as contention.
	if !llmReachable(cmd.Context()) {
		if used, err := gpuUsedMiB(cmd.Context()); err == nil && used > idleMiB {
			fmt.Fprintf(out, "GPU busy (%d MiB in use > %d, LLM not loaded) — skipping this pass\n", used, idleMiB)
			return nil
		}
	}

	for _, slug := range slugs {
		layout, err := world.Open(slug)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %v\n", slug, err)
			continue
		}
		if _, err := os.Stat(layout.WorldFile()); err != nil {
			fmt.Fprintf(out, "%s: no world.md — skipping\n", slug)
			continue
		}
		staged, _ := world.ListStaged(layout)
		if len(staged) >= maxStaged {
			fmt.Fprintf(out, "%s: %d proposals already awaiting review (>= %d) — skipping\n", slug, len(staged), maxStaged)
			continue
		}
		if dryRun {
			fmt.Fprintf(out, "%s: would expand %d thread(s) and stage for review\n", slug, threads)
			continue
		}
		fmt.Fprintf(out, "%s: GPU idle, expanding %d thread(s) (staging for review)...\n", slug, threads)
		if err := runExpand(cmd, slug, "", threads); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: expand failed: %v\n", slug, err)
			continue
		}
	}
	fmt.Fprintf(out, "\nautopilot pass done — review with: worldsmith expand review <slug>\n")
	return nil
}

// llmReachable reports whether the long_form LLM endpoint is up (any HTTP
// response, including 401, counts — the server is answering). curl exits 0 when
// it connected; non-zero (e.g. connection refused) means down.
func llmReachable(ctx context.Context) bool {
	return exec.CommandContext(ctx, "curl", "-sS", "-m", "2", "-o", "/dev/null", "http://127.0.0.1:9001/v1/models").Run() == nil
}

// gpuUsedMiB reads total VRAM in use via nvidia-smi.
func gpuUsedMiB(ctx context.Context) (int, error) {
	o, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0, err
	}
	// Sum across GPUs (usually one); take the first line's value.
	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(o)), "\n", 2)[0])
	return strconv.Atoi(first)
}
