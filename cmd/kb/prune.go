package kb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/durfmt"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagPrunePattern   string
	flagPruneOlderThan string
	flagPruneYes       bool
)

const prunePageSize = 100 // backend cap; internal detail, not exposed

// validatePruneFilters rejects "delete everything" — at least one of
// --pattern or --older-than must be set.
func validatePruneFilters(pattern, age string) error {
	if pattern == "" && age == "" {
		return clerr.Validation(i18n.T("kb.prune.no_filter"))
	}
	return nil
}

var pruneCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "prune",
	Short: i18n.T("kb.prune.short"),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validatePruneFilters(flagPrunePattern, flagPruneOlderThan); err != nil {
			return err
		}
		// Pre-flight the pattern syntax so a bad glob exits 2 before any
		// HTTP round-trip (the scan loop's per-page filterKBs() also
		// validates, but only after the first page has been fetched).
		if flagPrunePattern != "" {
			if _, err := filepath.Match(flagPrunePattern, ""); err != nil {
				return clerr.Validation(i18n.T("kb.prune.bad_pattern", flagPrunePattern, err.Error()))
			}
		}
		var olderThan time.Duration
		if flagPruneOlderThan != "" {
			d, err := durfmt.ParseAge(flagPruneOlderThan)
			if err != nil {
				return clerr.Validation(i18n.T("kb.prune.bad_age", flagPruneOlderThan, err.Error()))
			}
			olderThan = d
		}

		vc, err := cliauth.NewVectoriaClient()
		if err != nil {
			return err
		}

		// Scan all pages, accumulating matches.
		matched := make([]kbItem, 0, 64)
		offset := 0
		var total int
		now := time.Now()
		for {
			scanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			resp, err := vc.ListKBs(scanCtx, offset, prunePageSize)
			cancel()
			if err != nil {
				return err
			}
			total = resp.Total
			page := (offset / prunePageSize) + 1
			fmt.Fprintln(os.Stderr, i18n.T("kb.prune.scanning", page, offset+len(resp.Items), total))
			items := toItems(resp.Items)
			filtered, ferr := filterKBs(items, flagPrunePattern, olderThan, now)
			if ferr != nil {
				return clerr.Validation(i18n.T("kb.prune.bad_pattern", flagPrunePattern, ferr.Error()))
			}
			matched = append(matched, filtered...)
			offset += len(resp.Items)
			if offset >= total || len(resp.Items) == 0 {
				break
			}
		}

		format, _ := cmd.Flags().GetString("output")

		// Dry-run path (default).
		if !flagPruneYes {
			if format == "json" {
				outItems := make([]map[string]any, 0, len(matched))
				for _, k := range matched {
					outItems = append(outItems, map[string]any{
						"id":         k.ID,
						"name":       k.Name,
						"created_at": k.CreatedRaw,
						"status":     "would_delete",
					})
				}
				return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
					"dry_run": true,
					"matched": len(matched),
					"items":   outItems,
				})
			}
			fmt.Println(i18n.T("kb.prune.match_header", len(matched)))
			for _, k := range matched {
				fmt.Printf("  %s  %s  %s\n", k.ID, k.Name, k.CreatedRaw)
			}
			fmt.Println(i18n.T("kb.prune.dry_run_hint"))
			return nil
		}

		// Apply path.
		fmt.Fprintln(os.Stderr, i18n.T("kb.prune.applying", len(matched)))
		deleted := 0
		failed := 0
		outItems := make([]map[string]any, 0, len(matched))
		for _, k := range matched {
			delCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := vc.DeleteKB(delCtx, k.ID)
			cancel()
			status := "deleted"
			if err != nil {
				// 404 → idempotent success.
				if errs.HasCode(err, "not_found") {
					deleted++
					fmt.Fprintf(os.Stderr, "  ✓ %s  %s  (already gone)\n", k.ID, k.Name)
					outItems = append(outItems, map[string]any{
						"id":     k.ID,
						"name":   k.Name,
						"status": "deleted",
					})
					continue
				}
				failed++
				status = "failed"
				fmt.Fprintf(os.Stderr, "  ✗ %s  %s  (%s)\n", k.ID, k.Name, err.Error())
			} else {
				deleted++
				fmt.Fprintf(os.Stderr, "  ✓ %s  %s\n", k.ID, k.Name)
			}
			outItems = append(outItems, map[string]any{
				"id":     k.ID,
				"name":   k.Name,
				"status": status,
			})
		}
		fmt.Fprintln(os.Stderr, i18n.T("kb.prune.done", deleted, failed))

		if format == "json" {
			_ = output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"dry_run": false,
				"matched": len(matched),
				"deleted": deleted,
				"failed":  failed,
				"items":   outItems,
			})
		}

		// Exit codes: all-fail → 5, partial → 7, all-ok → 0.
		if failed > 0 && deleted == 0 {
			os.Exit(5)
		}
		if failed > 0 {
			os.Exit(7)
		}
		return nil
	},
}

func init() {
	pruneCmd.Flags().StringVar(&flagPrunePattern, "pattern", "", "glob matched against kb name (e.g., vibeknow-cli-*)")
	pruneCmd.Flags().StringVar(&flagPruneOlderThan, "older-than", "", "filter to kbs older than this duration (e.g., 7d)")
	pruneCmd.Flags().BoolVarP(&flagPruneYes, "yes", "y", false, "actually delete (default is dry-run)")
	Cmd.AddCommand(pruneCmd)
}
