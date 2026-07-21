package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hshare",
		Short: "HiveShare — collaborative AI memory CLI",
	}
	root.AddCommand(
		authCmd(),
		hiveshareCmd(),
		memoryCmd(),
		inviteCmd(),
		membersCmd(),
		streamCmd(),
		metricsCmd(),
	)
	return root
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func authCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authentication commands"}

	var email, name, serverURL string
	register := &cobra.Command{
		Use:   "register",
		Short: "Register a new account and save credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			if serverURL != "" {
				cfg.ServerURL = serverURL
			}
			c := &Client{BaseURL: cfg.ServerURL}
			var result map[string]interface{}
			if err := c.post("/api/v1/auth/register", map[string]string{
				"email": email, "name": name,
			}, &result); err != nil {
				return err
			}
			cfg.APIKey = result["api_key"].(string)
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("Registered! API key saved to ~/.config/hiveshare/config.json\n")
			fmt.Printf("User ID:  %s\n", result["id"])
			fmt.Printf("API Key:  %s\n", result["api_key"])
			return nil
		},
	}
	register.Flags().StringVar(&email, "email", "", "Email address (required)")
	register.Flags().StringVar(&name, "name", "", "Display name (required)")
	register.Flags().StringVar(&serverURL, "server", "", "Server URL (default http://localhost:8080)")
	register.MarkFlagRequired("email")
	register.MarkFlagRequired("name")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show current auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			var result map[string]interface{}
			if err := c.get("/api/v1/auth/whoami", &result); err != nil {
				return err
			}
			fmt.Printf("Logged in as: %s <%s>\n", result["name"], result["email"])
			fmt.Printf("Server:       %s\n", c.BaseURL)
			cfg := loadConfig()
			if cfg.DefaultHSName != "" {
				fmt.Printf("Hiveshare:    %s (%s)\n", cfg.DefaultHSName, cfg.DefaultHiveshare)
			}
			return nil
		},
	}

	cmd.AddCommand(register, status)
	return cmd
}

// ── Hiveshare ─────────────────────────────────────────────────────────────────

func hiveshareCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hiveshare", Short: "Manage hiveshares", Aliases: []string{"hs"}}

	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new hiveshare",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("description")
			c, err := newClient()
			if err != nil {
				return err
			}
			var result map[string]interface{}
			if err := c.post("/api/v1/hiveshares", map[string]string{
				"name": args[0], "description": desc,
			}, &result); err != nil {
				return err
			}
			fmt.Printf("Created hiveshare: %s (%s)\n", result["name"], result["id"])
			fmt.Printf("Run 'hshare hiveshare use %s' to set it as default\n", result["id"])
			return nil
		},
	}
	create.Flags().StringP("description", "d", "", "Description")

	list := &cobra.Command{
		Use:   "list",
		Short: "List all your hiveshares",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			var result []map[string]interface{}
			if err := c.get("/api/v1/hiveshares", &result); err != nil {
				return err
			}
			cfg := loadConfig()
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tID\tROLE\tMEMBERS\tACTIVE")
			for _, hs := range result {
				active := ""
				if hs["id"] == cfg.DefaultHiveshare {
					active = "*"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%.0f\t%s\n",
					hs["name"], hs["id"], hs["role"], hs["member_count"], active)
			}
			w.Flush()
			return nil
		},
	}

	use := &cobra.Command{
		Use:   "use <id>",
		Short: "Set a hiveshare as the default",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			var hs map[string]interface{}
			if err := c.get("/api/v1/hiveshares/"+args[0], &hs); err != nil {
				return err
			}
			cfg := loadConfig()
			cfg.DefaultHiveshare = args[0]
			cfg.DefaultHSName, _ = hs["name"].(string)
			if err := saveConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("Active hiveshare: %s (%s)\n", cfg.DefaultHSName, cfg.DefaultHiveshare)
			return nil
		},
	}

	cmd.AddCommand(create, list, use)
	return cmd
}

// ── Memory ────────────────────────────────────────────────────────────────────

func memoryCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Manage memory entries", Aliases: []string{"mem"}}

	add := &cobra.Command{
		Use:   "add",
		Short: "Add a memory entry (reads content from stdin or --content)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set; use --hiveshare or 'hshare hiveshare use <id>'")
			}
			sourceType, _ := cmd.Flags().GetString("source-type")
			sourceRef, _ := cmd.Flags().GetString("source-ref")
			sourceURL, _ := cmd.Flags().GetString("source-url")
			tool, _ := cmd.Flags().GetString("tool")
			summary, _ := cmd.Flags().GetString("summary")
			tagsStr, _ := cmd.Flags().GetString("tags")
			content, _ := cmd.Flags().GetString("content")

			if content == "" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				content = strings.TrimSpace(string(data))
			}
			if content == "" {
				return fmt.Errorf("content is required (pass --content or pipe via stdin)")
			}

			var tags []string
			if tagsStr != "" {
				for _, t := range strings.Split(tagsStr, ",") {
					tags = append(tags, strings.TrimSpace(t))
				}
			}

			var result map[string]interface{}
			err = c.post("/api/v1/hiveshares/"+hsID+"/memory", map[string]interface{}{
				"source_type": sourceType,
				"source_ref":  sourceRef,
				"source_url":  sourceURL,
				"tool":        tool,
				"content":     content,
				"summary":     summary,
				"tags":        tags,
			}, &result)
			if err != nil {
				return err
			}
			fmt.Printf("Memory entry added: %s\n", result["id"])
			if s, ok := result["summary"].(string); ok && s != "" {
				fmt.Printf("Summary: %s\n", s)
			}
			return nil
		},
	}
	add.Flags().String("hiveshare", "", "Hiveshare ID (uses default if not set)")
	add.Flags().StringP("source-type", "t", "manual", "Source type: jira, github_issue, github_pr, file, url, manual")
	add.Flags().StringP("source-ref", "r", "", "Source reference (e.g. PROJ-123)")
	add.Flags().String("source-url", "", "URL to the source")
	add.Flags().String("tool", "manual", "Tool used: claude, cursor, manual")
	add.Flags().String("summary", "", "Short summary")
	add.Flags().String("tags", "", "Comma-separated tags")
	add.Flags().String("content", "", "Content (or pipe via stdin)")
	add.MarkFlagRequired("source-ref")

	search := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memory entries",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set")
			}
			limit, _ := cmd.Flags().GetInt("limit")
			sourceType, _ := cmd.Flags().GetString("source-type")

			var result map[string]interface{}
			if err := c.post("/api/v1/hiveshares/"+hsID+"/memory/search", map[string]interface{}{
				"query":       strings.Join(args, " "),
				"source_type": sourceType,
				"limit":       limit,
			}, &result); err != nil {
				return err
			}

			entries, _ := result["results"].([]interface{})
			fmt.Printf("Found %d results for '%s'\n\n", len(entries), result["query"])
			for i, e := range entries {
				entry, _ := e.(map[string]interface{})
				fmt.Printf("[%d] %s / %s  (by %s, via %s)\n",
					i+1, entry["source_type"], entry["source_ref"], entry["user_name"], entry["tool"])
				if s, ok := entry["summary"].(string); ok && s != "" {
					fmt.Printf("    Summary: %s\n", s)
				}
				score, _ := entry["score"].(float64)
				fmt.Printf("    Score: %.3f | Views: %.0f | Reuses: %.0f\n\n",
					score, entry["views"], entry["reuses"])
			}
			return nil
		},
	}
	search.Flags().String("hiveshare", "", "Hiveshare ID")
	search.Flags().IntP("limit", "l", 10, "Max results")
	search.Flags().StringP("source-type", "t", "", "Filter by source type")

	list := &cobra.Command{
		Use:   "list",
		Short: "List memory entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set")
			}
			limit, _ := cmd.Flags().GetInt("limit")
			sourceType, _ := cmd.Flags().GetString("source-type")

			path := fmt.Sprintf("/api/v1/hiveshares/%s/memory?limit=%d", hsID, limit)
			if sourceType != "" {
				path += "&source_type=" + sourceType
			}

			var entries []map[string]interface{}
			if err := c.get(path, &entries); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SOURCE\tREF\tTOOL\tBY\tVIEWS\tREUSES\tDATE")
			for _, e := range entries {
				t, _ := time.Parse(time.RFC3339Nano, e["created_at"].(string))
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.0f\t%.0f\t%s\n",
					e["source_type"], e["source_ref"], e["tool"], e["user_name"],
					e["views"], e["reuses"], t.Format("2006-01-02"))
			}
			w.Flush()
			return nil
		},
	}
	list.Flags().String("hiveshare", "", "Hiveshare ID")
	list.Flags().IntP("limit", "l", 20, "Max results")
	list.Flags().StringP("source-type", "t", "", "Filter by source type")

	cmd.AddCommand(add, search, list)
	return cmd
}

// ── Invite ────────────────────────────────────────────────────────────────────

func inviteCmd() *cobra.Command {
	var role, hsID string
	cmd := &cobra.Command{
		Use:   "invite <email>",
		Short: "Invite a collaborator to the current hiveshare",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set")
			}
			var result map[string]interface{}
			if err := c.post("/api/v1/hiveshares/"+hsID+"/invite", map[string]string{
				"email": args[0], "role": role,
			}, &result); err != nil {
				return err
			}
			fmt.Printf("Invitation sent to %s\n", args[0])
			fmt.Printf("Invite link: %s\n", result["invite_url"])
			fmt.Printf("Role: %s | Expires: %s\n", result["role"], result["expires_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "member", "Role: member or viewer")
	cmd.Flags().StringVar(&hsID, "hiveshare", "", "Hiveshare ID (uses default)")
	return cmd
}

// ── Members ───────────────────────────────────────────────────────────────────

func membersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "members", Short: "Manage hiveshare members"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List members of the current hiveshare",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			var members []map[string]interface{}
			if err := c.get("/api/v1/hiveshares/"+hsID+"/members", &members); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tEMAIL\tROLE\tJOINED")
			for _, m := range members {
				t, _ := time.Parse(time.RFC3339Nano, m["joined_at"].(string))
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m["name"], m["email"], m["role"], t.Format("2006-01-02"))
			}
			w.Flush()
			return nil
		},
	}
	list.Flags().String("hiveshare", "", "Hiveshare ID")
	cmd.AddCommand(list)
	return cmd
}

// ── Stream ────────────────────────────────────────────────────────────────────

func streamCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stream",
		Short: "Stream live updates from the current hiveshare",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set")
			}

			fmt.Printf("Streaming updates from hiveshare %s (Ctrl+C to stop)...\n\n", hsID)
			req, _ := http.NewRequest(http.MethodGet,
				c.BaseURL+"/api/v1/hiveshares/"+hsID+"/stream", nil)
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
			req.Header.Set("Accept", "text/event-stream")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			var eventType string
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "event: ") {
					eventType = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					var payload map[string]interface{}
					if json.Unmarshal([]byte(data), &payload) == nil {
						ts := time.Now().Format("15:04:05")
						switch eventType {
						case "memory_added":
							p, _ := payload["payload"].(map[string]interface{})
							fmt.Printf("[%s] + memory added: %s/%s by %s\n",
								ts, p["source_type"], p["source_ref"], p["user_name"])
						case "memory_updated":
							p, _ := payload["payload"].(map[string]interface{})
							fmt.Printf("[%s] ~ memory updated: %s/%s\n",
								ts, p["source_type"], p["source_ref"])
						case "connected":
							fmt.Printf("[%s] connected to stream\n", ts)
						default:
							fmt.Printf("[%s] %s: %s\n", ts, eventType, data)
						}
					}
					eventType = ""
				}
			}
			return scanner.Err()
		},
	}
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func metricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show hiveshare or personal metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient()
			if err != nil {
				return err
			}
			me, _ := cmd.Flags().GetBool("me")
			if me {
				var result map[string]interface{}
				if err := c.get("/api/v1/metrics/me", &result); err != nil {
					return err
				}
				fmt.Println("── Personal Metrics ──────────────────")
				fmt.Printf("Entries contributed:  %.0f\n", result["total_entries"])
				fmt.Printf("Searches performed:   %.0f\n", result["total_searches"])
				fmt.Printf("Headspaces owned:     %.0f\n", result["hiveshares_owned"])
				fmt.Printf("Headspaces joined:    %.0f\n", result["hiveshares_joined"])
				fmt.Printf("Total reuses given:   %.0f\n", result["total_reuses_given"])
				return nil
			}

			cfg := loadConfig()
			hsID, _ := cmd.Flags().GetString("hiveshare")
			if hsID == "" {
				hsID = cfg.DefaultHiveshare
			}
			if hsID == "" {
				return fmt.Errorf("no hiveshare set; use --hiveshare or 'hshare hiveshare use <id>'")
			}

			var m map[string]interface{}
			if err := c.get("/api/v1/hiveshares/"+hsID+"/metrics", &m); err != nil {
				return err
			}

			hs, _ := m["hiveshare"].(map[string]interface{})
			mem, _ := m["memory"].(map[string]interface{})
			collab, _ := m["collaboration"].(map[string]interface{})
			coverage, _ := m["coverage"].(map[string]interface{})
			activity, _ := m["activity"].(map[string]interface{})

			fmt.Printf("── %s ────────────────────────────────────\n", hs["name"])
			fmt.Printf("Members: %.0f\n\n", hs["member_count"])

			fmt.Println("Memory")
			fmt.Printf("  Total entries:    %.0f\n", mem["total_entries"])
			fmt.Printf("  Unique sources:   %.0f\n", mem["unique_sources"])
			if bt, ok := mem["by_source_type"].(map[string]interface{}); ok {
				for k, v := range bt {
					fmt.Printf("  %-18s %.0f\n", k+":", v)
				}
			}

			fmt.Println("\nCollaboration")
			fmt.Printf("  Total views:      %.0f\n", collab["total_views"])
			fmt.Printf("  Total reuses:     %.0f\n", collab["total_reuses"])
			fmt.Printf("  Reuse rate:       %.1f%%\n", collab["reuse_rate"].(float64)*100)

			fmt.Println("\nCoverage")
			fmt.Printf("  Jira refs:        %.0f\n", coverage["jira_refs_with_memory"])
			fmt.Printf("  GitHub refs:      %.0f\n", coverage["github_refs_with_memory"])

			fmt.Println("\nActivity (last 7d)")
			fmt.Printf("  Adds:             %.0f\n", activity["last_7d_adds"])
			fmt.Printf("  Searches:         %.0f\n", activity["last_7d_searches"])
			fmt.Printf("  Active users:     %.0f\n", activity["active_users_7d"])

			if contribs, ok := collab["top_contributors"].([]interface{}); ok && len(contribs) > 0 {
				fmt.Println("\nTop Contributors")
				for _, ci := range contribs {
					c, _ := ci.(map[string]interface{})
					fmt.Printf("  %-20s entries:%.0f  reuses:%.0f\n",
						c["name"], c["entries"], c["reuses_received"])
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("me", false, "Show personal metrics instead of hiveshare metrics")
	cmd.Flags().String("hiveshare", "", "Hiveshare ID (uses default)")
	return cmd
}
