package cli

import (
	"fmt"

	"github.com/Valaraucoo/wolt-cli/internal/domain"
	"github.com/spf13/cobra"
)

func newConfigureCommand(deps Dependencies) *cobra.Command {
	var profileName string
	var address string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Create and manage local profile configuration.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !overwrite {
				if _, err := deps.Config.Load(cmd.Context()); err == nil {
					return fmt.Errorf("config file already exists, rerun with --overwrite")
				}
			}

			location, err := deps.Location.Get(cmd.Context(), address)
			if err != nil {
				return err
			}
			cfg := domain.Config{
				Profiles: []domain.Profile{
					{
						Name:      profileName,
						IsDefault: true,
						Address:   address,
						Location:  location,
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
	cmd.Flags().StringVar(&address, "address", "", "Profile address")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing config")
	if err := cmd.MarkFlagRequired("address"); err != nil {
		panic(err)
	}
	return cmd
}
