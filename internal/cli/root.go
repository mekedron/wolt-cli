package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewRootCommand builds the complete command tree.
func NewRootCommand(deps Dependencies) *cobra.Command {
	version := deps.Version
	if strings.TrimSpace(version) == "" {
		version = "1.1.1"
	}

	root := &cobra.Command{
		Use:           "wolt-cli",
		Short:         "Browse Wolt venues, inspect menus, and manage local profiles.",
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			showVersion, _ := cmd.Flags().GetBool("version")
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), version)
				return errVersionShown
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			showVersion, _ := cmd.Flags().GetBool("version")
			if !showVersion {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return errVersionShown
		},
	}
	root.Flags().BoolP("version", "v", false, "Show CLI version and exit.")
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	defaultHelpFunc := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root {
			renderRootHelp(cmd.OutOrStdout(), root)
			return
		}
		defaultHelpFunc(cmd, args)
	})

	root.AddCommand(newDiscoverCommand(deps))
	root.AddCommand(newSearchCommand(deps))
	root.AddCommand(newVenueCommand(deps))
	root.AddCommand(newItemCommand(deps))
	root.AddCommand(newConfigureCommand(deps))

	return root
}

func renderRootHelp(out io.Writer, root *cobra.Command) {
	fmt.Fprintf(out, "%s: %s\n\n", root.Name(), root.Short)
	fmt.Fprintf(out, "usage: %s <command> [options]\n", root.Name())
	fmt.Fprintln(out, "global options:")
	for _, token := range rootOptionTokens(root) {
		fmt.Fprintf(out, "  %s\n", token)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "commands:")
	for _, cmd := range visibleCommands(root) {
		fmt.Fprintf(out, "  %s\n", cmd.Name())
		fmt.Fprintf(out, "    %s\n", cmd.Short)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "full reference:")
	emitReference(out, root, root.Name())
}

func rootOptionTokens(root *cobra.Command) []string {
	options := []string{}
	root.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Name == "help" {
			return
		}
		token := "--" + flag.Name
		if flag.Shorthand != "" {
			token += "/-" + flag.Shorthand
		}
		options = append(options, token)
	})
	sort.Strings(options)
	return options
}

func visibleCommands(parent *cobra.Command) []*cobra.Command {
	commands := make([]*cobra.Command, 0)
	for _, cmd := range parent.Commands() {
		if cmd.Hidden {
			continue
		}
		commands = append(commands, cmd)
	}
	return commands
}

func emitReference(out io.Writer, parent *cobra.Command, path string) {
	for _, cmd := range visibleCommands(parent) {
		signature := strings.TrimSpace(path + " " + cmd.Use)
		flags := optionTokens(cmd)
		if len(flags) > 0 {
			signature = signature + " " + strings.Join(flags, " ")
		}
		fmt.Fprintf(out, "- %s\n", signature)
		fmt.Fprintf(out, "  %s\n\n", cmd.Short)
		emitReference(out, cmd, strings.TrimSpace(path+" "+cmd.Name()))
	}
}

func optionTokens(cmd *cobra.Command) []string {
	tokens := []string{}
	cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Name == "help" {
			return
		}
		tokens = append(tokens, "[--"+flag.Name+"]")
	})
	sort.Strings(tokens)
	return tokens
}
