package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type report struct {
	models         []string
	variants       []requestVariant
	resultsByModel map[string]map[string]testResult
	totalRequests  int
	failedCount    int
	skippedCount   int
}

// buildReport aggregates raw test results by model and variant.
func buildReport(models []string, variants []requestVariant, results []testResult) report {
	byModel := make(map[string]map[string]testResult, len(models))
	for _, model := range models {
		byModel[model] = make(map[string]testResult)
	}

	failed := 0
	skipped := 0
	for _, res := range results {
		if res.Model == "" {
			continue
		}
		modelMap, ok := byModel[res.Model]
		if !ok {
			modelMap = make(map[string]testResult)
			byModel[res.Model] = modelMap
		}
		modelMap[res.RequestFormat] = res
		if res.Skipped {
			skipped++
			continue
		}
		if !res.Success {
			failed++
		}
	}

	return report{
		models:         models,
		variants:       variants,
		resultsByModel: byModel,
		totalRequests:  len(results),
		failedCount:    failed,
		skippedCount:   skipped,
	}
}

// renderReport prints the matrix view and summaries to stdout.
func renderReport(rep report) {
	if len(rep.models) == 0 {
		fmt.Println("no models to report")
		return
	}
	if len(rep.variants) == 0 {
		fmt.Println("no api formats selected")
		return
	}

	fmt.Println()
	fmt.Printf("=== One-API Compatibility Matrix %s ===\n", time.Now().Format(time.RFC3339))
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Request Format")
	for _, model := range rep.models {
		fmt.Fprintf(tw, "\t%s", model)
	}
	fmt.Fprintln(tw)

	for _, variant := range rep.variants {
		fmt.Fprintf(tw, "%s", variant.Header)
		for _, model := range rep.models {
			entry := rep.resultsByModel[model]
			cell := formatMatrixCell(entry[variant.Key])
			fmt.Fprintf(tw, "\t%s", cell)
		}
		fmt.Fprintln(tw)
	}
	_ = tw.Flush()

	fmt.Println()

	passed := rep.totalRequests - rep.failedCount - rep.skippedCount
	fmt.Printf("Totals  | Requests: %d | Passed: %d | Failed: %d | Skipped: %d\n",
		rep.totalRequests,
		passed,
		rep.failedCount,
		rep.skippedCount,
	)

	failures, skips := gatherOutcomes(rep)
	warnings := gatherWarnings(rep)
	if len(failures) > 0 {
		fmt.Println()
		fmt.Println("Failures:")
		for _, res := range failures {
			fmt.Printf("- %s - %s -> %s\n", res.Model, res.Label, shorten(res.ErrorReason, 200))
		}
	}
	if len(warnings) > 0 {
		fmt.Println()
		fmt.Println("Warnings (passed with caveats):")
		for _, res := range warnings {
			fmt.Printf("- %s - %s -> %s\n", res.Model, res.Label, shorten(res.Warning, 200))
		}
	}
	if len(skips) > 0 {
		fmt.Println()
		fmt.Println("Skipped (unsupported combinations):")
		for _, res := range skips {
			fmt.Printf("- %s - %s -> %s\n", res.Model, res.Label, shorten(res.ErrorReason, 200))
		}
	}

	fmt.Println()
}

func formatMatrixCell(res testResult) string {
	if res.Model == "" {
		return "—"
	}

	duration := res.Duration.Truncate(10 * time.Millisecond)
	switch {
	case res.Success:
		if strings.TrimSpace(res.Warning) != "" {
			return fmt.Sprintf("PASS* %.2fs", duration.Seconds())
		}
		return fmt.Sprintf("PASS %.2fs", duration.Seconds())
	case res.Skipped:
		reason := res.ErrorReason
		if reason == "" {
			reason = "skipped"
		}
		return "SKIP"
	default:
		reason := res.ErrorReason
		if reason == "" {
			reason = duration.String()
		}
		return fmt.Sprintf("FAIL %s", shorten(reason, 32))
	}
}

// gatherWarnings collects results that passed but carry warning messages.
func gatherWarnings(rep report) []testResult {
	var warnings []testResult
	for _, model := range rep.models {
		entry := rep.resultsByModel[model]
		for _, variant := range rep.variants {
			res, ok := entry[variant.Key]
			if !ok || res.Model == "" {
				continue
			}
			if res.Success && strings.TrimSpace(res.Warning) != "" {
				warnings = append(warnings, res)
			}
		}
	}
	return warnings
}

func gatherOutcomes(rep report) (failures, skips []testResult) {
	for _, model := range rep.models {
		entry := rep.resultsByModel[model]
		for _, variant := range rep.variants {
			res, ok := entry[variant.Key]
			if !ok || res.Model == "" {
				continue
			}
			if res.Skipped {
				skips = append(skips, res)
				continue
			}
			if !res.Success {
				failures = append(failures, res)
			}
		}
	}

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Model == failures[j].Model {
			return failures[i].Label < failures[j].Label
		}
		return failures[i].Model < failures[j].Model
	})
	sort.Slice(skips, func(i, j int) bool {
		if skips[i].Model == skips[j].Model {
			return skips[i].Label < skips[j].Label
		}
		return skips[i].Model < skips[j].Model
	})

	return failures, skips
}
