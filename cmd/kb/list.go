package kb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/client/vectoria"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/durfmt"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
)

var (
	flagListPage      int
	flagListSize      int
	flagListPattern   string
	flagListOlderThan string
)

// kbItem is the display-side struct: vectoria.KB fields + parsed CreatedAt.
type kbItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"-"`
	CreatedRaw  string    `json:"created_at"`
}

func toItems(in []vectoria.KB) []kbItem {
	out := make([]kbItem, 0, len(in))
	for _, k := range in {
		t, _ := time.Parse(time.RFC3339, k.CreatedAt)
		out = append(out, kbItem{
			ID:          k.ID,
			Name:        k.Name,
			Description: k.Description,
			CreatedAt:   t,
			CreatedRaw:  k.CreatedAt,
		})
	}
	return out
}

// filterKBs applies pattern (filepath.Match glob) and age-min filters.
// pattern == "" → no pattern filter. olderThan == 0 → no age filter.
// `now` is injected for deterministic tests.
func filterKBs(items []kbItem, pattern string, olderThan time.Duration, now time.Time) ([]kbItem, error) {
	if pattern != "" {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, err
		}
	}
	out := make([]kbItem, 0, len(items))
	for _, k := range items {
		if pattern != "" {
			ok, _ := filepath.Match(pattern, k.Name)
			if !ok {
				continue
			}
		}
		if olderThan > 0 && !k.CreatedAt.IsZero() {
			if now.Sub(k.CreatedAt) < olderThan {
				continue
			}
		}
		out = append(out, k)
	}
	return out, nil
}

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: i18n.T("kb.list.short"),
	RunE: func(cmd *cobra.Command, args []string) error {
		var olderThan time.Duration
		if flagListOlderThan != "" {
			d, err := durfmt.ParseAge(flagListOlderThan)
			if err != nil {
				return clerr.Validation(i18n.T("kb.prune.bad_age", flagListOlderThan, err.Error()))
			}
			olderThan = d
		}
		// Pre-flight pattern validity so a bad glob exits 2 before any HTTP.
		if flagListPattern != "" {
			if _, err := filepath.Match(flagListPattern, ""); err != nil {
				return clerr.Validation(i18n.T("kb.prune.bad_pattern", flagListPattern, err.Error()))
			}
		}

		vc, err := cliauth.NewVectoriaClient()
		if err != nil {
			return err
		}
		offset := (flagListPage - 1) * flagListSize
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := vc.ListKBs(ctx, offset, flagListSize)
		if err != nil {
			return err
		}
		items := toItems(resp.Items)
		filtered, err := filterKBs(items, flagListPattern, olderThan, time.Now())
		if err != nil {
			// Pre-flighted above; reaching here means a non-pattern issue.
			return err
		}

		format, _ := cmd.Flags().GetString("output")
		if format == "json" {
			return output.NewJSON(cmd.OutOrStdout()).Object(map[string]any{
				"page":  flagListPage,
				"size":  flagListSize,
				"total": resp.Total,
				"kbs":   filtered,
			})
		}
		if len(filtered) == 0 {
			fmt.Println(i18n.T("kb.list.empty"))
			return nil
		}
		fmt.Printf("%-38s  %-30s  %-19s  %s\n", "ID", "NAME", "CREATED", "DESCRIPTION")
		fmt.Println(strings.Repeat("-", 110))
		for _, k := range filtered {
			name := k.Name
			if len(name) > 28 {
				name = name[:28] + ".."
			}
			created := "--"
			if !k.CreatedAt.IsZero() {
				created = k.CreatedAt.Local().Format("2006-01-02 15:04")
			}
			fmt.Printf("%-38s  %-30s  %-19s  %s\n", k.ID, name, created, k.Description)
		}
		fmt.Println(i18n.T("kb.list.footer", len(filtered), resp.Total, flagListPage))
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&flagListPage, "page", 1, "page number")
	listCmd.Flags().IntVar(&flagListSize, "size", 50, "page size (backend caps at 100)")
	listCmd.Flags().StringVar(&flagListPattern, "pattern", "", "glob pattern matched against kb name (filepath.Match syntax)")
	listCmd.Flags().StringVar(&flagListOlderThan, "older-than", "", "filter to kbs older than this duration (e.g., 7d, 24h, 1h30m)")
	// `jobs list` spells the row cap --limit; accept it here too.
	cmdutil.AliasFlags(listCmd, map[string]string{"limit": "size"})
	Cmd.AddCommand(listCmd)
}
