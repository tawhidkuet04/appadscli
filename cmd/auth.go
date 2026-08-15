package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tawhidkuet04/adastra/internal/api"
	"github.com/tawhidkuet04/adastra/internal/auth"
	"github.com/tawhidkuet04/adastra/internal/config"
)

func init() {
	authCmd := &cobra.Command{Use: "auth", Short: "Login, status, and diagnostics for Apple Ads API credentials"}

	var (
		clientID, teamID, keyID, keyPath string
		bypassKeychain                   bool
	)
	login := &cobra.Command{
		Use:   "login",
		Short: "Store Apple Ads API credentials (keychain on macOS, 0600 file elsewhere)",
		Long: `Store your Apple Ads API credentials.

Create them in Apple Ads → Account Settings → API: generate an EC P-256 key
pair, upload the public key, and note the clientId, teamId, and keyId.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pem, err := os.ReadFile(keyPath)
			if err != nil {
				return fmt.Errorf("reading --private-key: %w", err)
			}
			creds := &auth.Credentials{ClientID: clientID, TeamID: teamID, KeyID: keyID, PrivateKey: string(pem)}
			if err := auth.VerifyKey(creds); err != nil {
				return fmt.Errorf("private key check failed: %w", err)
			}
			where, err := auth.SaveCredentials(creds, bypassKeychain)
			if err != nil {
				return err
			}
			c := cfg()
			c.BypassKeychain = bypassKeychain
			if err := config.Save(c); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "✓ credentials stored (%s)\n", where)
			if _, err := auth.Refresh(cmd.Context()); err != nil {
				fmt.Fprintln(os.Stderr, "⚠ token exchange failed — credentials saved, but check them with `adastra auth doctor`:")
				return err
			}
			fmt.Fprintln(os.Stderr, "✓ token exchange OK — you're logged in")
			fmt.Fprintln(os.Stderr, "next: `adastra accounts list` then `adastra accounts use <id>`")
			return nil
		},
	}
	login.Flags().StringVar(&clientID, "client-id", "", "Apple Ads API clientId (required)")
	login.Flags().StringVar(&teamID, "team-id", "", "Apple Ads API teamId (required)")
	login.Flags().StringVar(&keyID, "key-id", "", "Apple Ads API keyId (required)")
	login.Flags().StringVar(&keyPath, "private-key", "", "path to EC P-256 private key .pem (required)")
	login.Flags().BoolVar(&bypassKeychain, "bypass-keychain", false, "store credentials on disk instead of macOS keychain")
	_ = login.MarkFlagRequired("client-id")
	_ = login.MarkFlagRequired("team-id")
	_ = login.MarkFlagRequired("key-id")
	_ = login.MarkFlagRequired("private-key")

	var validate bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Show login state and cached token expiry",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := auth.LoadCredentials()
			if err != nil {
				return err
			}
			out := map[string]any{
				"clientId": creds.ClientID,
				"teamId":   creds.TeamID,
				"keyId":    creds.KeyID,
				"account":  cfg().DefaultAccount,
			}
			if validate {
				tok, err := auth.Refresh(cmd.Context())
				if err != nil {
					return err
				}
				out["tokenValid"] = true
				out["tokenExpires"] = tok.ExpiresAt.Format(time.RFC3339)
				var me json.RawMessage
				if err := client().Get(cmd.Context(), "/v1/me", &me); err == nil {
					out["me"] = me
				}
			}
			return render().JSON(out)
		},
	}
	status.Flags().BoolVar(&validate, "validate", false, "exchange a fresh token and call /v1/me")

	doctor := &cobra.Command{
		Use:   "doctor",
		Short: "Full diagnostic: key, token exchange, org access, ad account context",
		RunE: func(cmd *cobra.Command, args []string) error {
			ok := true
			step := func(name string, err error) {
				if err != nil {
					ok = false
					fmt.Printf("✗ %-28s %v\n", name, err)
				} else {
					fmt.Printf("✓ %s\n", name)
				}
			}
			creds, err := auth.LoadCredentials()
			step("credentials stored", err)
			if err != nil {
				return fmt.Errorf("run `adastra auth login` first")
			}
			step("private key parses & signs", auth.VerifyKey(creds))
			_, tokErr := auth.Refresh(cmd.Context())
			step("OAuth2 token exchange", tokErr)
			if tokErr == nil {
				c := client()
				var me json.RawMessage
				step("GET /v1/me", c.Get(cmd.Context(), "/v1/me", &me))
				var acls []json.RawMessage
				aclErr := c.Get(cmd.Context(), "/v1/acls", &acls)
				step("GET /v1/acls (org access)", aclErr)
				if aclErr == nil {
					fmt.Printf("  → %d ad account(s) accessible\n", len(acls))
				}
				if c.AdAccount == "" {
					ok = false
					fmt.Println("✗ default ad account          none set — run `adastra accounts use <id>`")
				} else {
					fmt.Printf("✓ default ad account          %s\n", c.AdAccount)
				}
			}
			if !ok {
				return fmt.Errorf("doctor found problems")
			}
			fmt.Println("all checks passed")
			return nil
		},
	}

	logout := &cobra.Command{
		Use:   "logout",
		Short: "Delete stored credentials and cached tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.DeleteCredentials(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "✓ logged out")
			return nil
		},
	}

	authCmd.AddCommand(login, status, doctor, logout)
	rootCmd.AddCommand(authCmd)

	// accounts + me
	accountsCmd := &cobra.Command{Use: "accounts", Short: "List and select Apple Ads ad accounts"}
	list := &cobra.Command{
		Use:   "list",
		Short: "Ad accounts you can access (from /v1/acls)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var acls []json.RawMessage
			if err := client().Get(cmd.Context(), "/v1/acls", &acls); err != nil {
				return err
			}
			h, rows := api.Table(acls, []string{
				"AdAccountId=adAccountId", "OrgId=orgId", "Name=orgName",
				"Currency=currency", "TimeZone=timeZone", "Roles=roleNames", "PaymentModel=paymentModel",
			})
			return render().Rows(h, rows, acls)
		},
	}
	use := &cobra.Command{
		Use:   "use <adAccountId>",
		Short: "Set the default ad account for scoped commands",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := cfg()
			c.DefaultAccount = args[0]
			if err := config.Save(c); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "✓ default ad account set to", args[0])
			return nil
		},
	}
	accountsCmd.AddCommand(list, use)
	rootCmd.AddCommand(accountsCmd)

	meCmd := &cobra.Command{
		Use:   "me",
		Short: "Identity, org, and roles for the current credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			var me json.RawMessage
			if err := client().Get(cmd.Context(), "/v1/me", &me); err != nil {
				return err
			}
			return render().JSON(me)
		},
	}
	rootCmd.AddCommand(meCmd)
}
