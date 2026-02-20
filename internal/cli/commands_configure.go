package cli

import (
	"fmt"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
	"github.com/spf13/cobra"
)

func newConfigureCommand(deps Dependencies) *cobra.Command {
	var profileName string
	var wtoken string
	var wrefreshToken string
	var cookies []string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Create and manage local profile auth configuration.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cookieInputs := normalizeCookieInputs(cookies)
			refreshCandidate := extractRefreshToken(wrefreshToken)
			if refreshCandidate == "" {
				refreshCandidate = normalizeRefreshToken(wrefreshToken)
			}
			if refreshCandidate == "" {
				refreshCandidate = extractRefreshToken(wtoken)
			}
			if refreshCandidate == "" {
				refreshCandidate = extractRefreshTokenFromCookieInputs(cookieInputs)
			}

			existingCfg, loadErr := deps.Config.Load(cmd.Context())
			hasExisting := loadErr == nil
			if hasExisting && !overwrite {
				if strings.TrimSpace(wtoken) == "" && strings.TrimSpace(refreshCandidate) == "" && len(cookieInputs) == 0 {
					return fmt.Errorf("provide --wtoken, --wrtoken, or --cookie to update auth fields")
				}
				index := findProfileIndex(existingCfg, profileName)
				if index < 0 {
					return fmt.Errorf("profile %q not found in existing config", profileName)
				}
				if strings.TrimSpace(wtoken) != "" {
					existingCfg.Profiles[index].WToken = normalizeWToken(wtoken)
				}
				if strings.TrimSpace(refreshCandidate) != "" {
					existingCfg.Profiles[index].WRefreshToken = refreshCandidate
				}
				if len(cookieInputs) > 0 {
					existingCfg.Profiles[index].Cookies = cookieInputs
				}
				if err := deps.Config.Save(cmd.Context(), existingCfg); err != nil {
					return err
				}
				return writeTable(cmd, "🏁 Config auth updated successfully!", "")
			}

			cfg := domain.Config{
				Profiles: []domain.Profile{
					{
						Name:          profileName,
						IsDefault:     true,
						WToken:        normalizeWToken(wtoken),
						WRefreshToken: refreshCandidate,
						Cookies:       cookieInputs,
					},
				},
			}
			if err := deps.Config.Save(cmd.Context(), cfg); err != nil {
				return err
			}
			return writeTable(cmd, "🏁 Config was created successfully!", "")
		},
	}

	cmd.Flags().StringVar(&profileName, "profile-name", "Default", "Profile name")
	cmd.Flags().StringVar(&wtoken, "wtoken", "", "Optional auth token saved with the profile for authenticated commands.")
	cmd.Flags().StringVar(&wrefreshToken, "wrtoken", "", "Optional refresh token saved with the profile for automatic token rotation.")
	cmd.Flags().StringArrayVar(&cookies, "cookie", nil, "Optional cookie value saved with the profile (repeatable).")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing config")
	return cmd
}

func findProfileIndex(cfg domain.Config, profileName string) int {
	trimmed := strings.TrimSpace(profileName)
	if trimmed != "" {
		for i, profile := range cfg.Profiles {
			if strings.EqualFold(strings.TrimSpace(profile.Name), trimmed) {
				return i
			}
		}
	}
	for i, profile := range cfg.Profiles {
		if profile.IsDefault {
			return i
		}
	}
	if len(cfg.Profiles) == 1 {
		return 0
	}
	return -1
}
