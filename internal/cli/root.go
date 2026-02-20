package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var sharedGlobalOptionOrder = []string{
	"format",
	"profile",
	"address",
	"locale",
	"no-color",
	"wtoken",
	"wrtoken",
	"cookie",
	"verbose",
}

var sharedGlobalOptionIndex = func() map[string]int {
	index := make(map[string]int, len(sharedGlobalOptionOrder))
	for i, name := range sharedGlobalOptionOrder {
		index[name] = i
	}
	return index
}()

// NewRootCommand builds the complete command tree.
func NewRootCommand(deps Dependencies) *cobra.Command {
	version := resolvedVersion(deps.Version)

	root := &cobra.Command{
		Use:           "wolt",
		Short:         "Browse Wolt venues, inspect menus, and manage local profiles.",
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			showVersion, _ := cmd.Flags().GetBool("version")
			if showVersion {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), version)
				return errVersionShown
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			attachVerboseHTTPTrace(cmd, deps.Wolt)
			showVersion, _ := cmd.Flags().GetBool("version")
			if !showVersion {
				return nil
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), version)
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
	root.AddCommand(newAuthCommand(deps))
	root.AddCommand(newCartCommand(deps))
	root.AddCommand(newCheckoutCommand(deps))
	root.AddCommand(newProfileCommand(deps))
	root.AddCommand(newConfigureCommand(deps))

	return root
}

type verboseHTTPTraceSetter interface {
	SetVerboseOutput(out io.Writer)
}

func attachVerboseHTTPTrace(cmd *cobra.Command, upstream any) {
	if cmd == nil || upstream == nil {
		return
	}
	verbose, _ := cmd.Flags().GetBool("verbose")
	if !verbose {
		return
	}
	setter, ok := upstream.(verboseHTTPTraceSetter)
	if !ok {
		return
	}
	setter.SetVerboseOutput(cmd.ErrOrStderr())
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "[verbose] http trace enabled")
}

func renderRootHelp(out io.Writer, root *cobra.Command) {
	_, _ = fmt.Fprintf(out, "%s: %s\n\n", root.Name(), root.Short)
	_, _ = fmt.Fprintf(out, "usage: %s <command> [options]\n", root.Name())
	_, _ = fmt.Fprintln(out, "global options (all optional unless marked required):")
	for _, option := range rootOptions(root) {
		_, _ = fmt.Fprintf(out, "  %s%s: %s\n", option.token, optionLabels(option), option.usage)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "commands:")
	for _, cmd := range visibleCommands(root) {
		_, _ = fmt.Fprintf(out, "  %s\n", cmd.Name())
		_, _ = fmt.Fprintf(out, "    %s\n", cmd.Short)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "notes:")
	_, _ = fmt.Fprintln(out, "  - options are optional unless marked [required].")
	_, _ = fmt.Fprintln(out, "  - checkout is preview-only in this CLI; final order placement happens in Wolt using your account-selected delivery address.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "full reference:")
	emitReference(out, root, root.Name())
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
		_, _ = fmt.Fprintf(out, "- %s\n", signature)
		_, _ = fmt.Fprintf(out, "  %s\n", cmd.Short)
		options := commandOptions(cmd)
		if len(options) > 0 {
			_, _ = fmt.Fprintln(out, "  options:")
			for _, option := range options {
				_, _ = fmt.Fprintf(out, "    %s%s: %s\n", option.token, optionLabels(option), option.usage)
			}
		}
		_, _ = fmt.Fprintln(out)
		emitReference(out, cmd, strings.TrimSpace(path+" "+cmd.Name()))
	}
}

type optionDoc struct {
	name      string
	token     string
	usage     string
	required  bool
	inherited bool
	shared    bool
}

func rootOptions(root *cobra.Command) []optionDoc {
	options := collectOptionDocs(root.Flags(), false)
	options = append(options, discoverSharedGlobalOptions(root)...)
	return options
}

func commandOptions(cmd *cobra.Command) []optionDoc {
	seen := map[string]struct{}{}
	options := make([]optionDoc, 0)
	for _, option := range collectOptionDocs(cmd.NonInheritedFlags(), false) {
		if option.shared || (cmd.Name() == "configure" && isSharedGlobalOption(option.name)) {
			continue
		}
		seen[option.name] = struct{}{}
		options = append(options, option)
	}
	for _, option := range collectOptionDocs(cmd.InheritedFlags(), true) {
		if _, ok := seen[option.name]; ok {
			continue
		}
		if option.shared || (cmd.Name() == "configure" && isSharedGlobalOption(option.name)) {
			continue
		}
		options = append(options, option)
	}
	return options
}

func discoverSharedGlobalOptions(root *cobra.Command) []optionDoc {
	discovered := map[string]optionDoc{}
	var walk func(*cobra.Command)
	walk = func(parent *cobra.Command) {
		for _, cmd := range visibleCommands(parent) {
			cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
				if flag.Hidden || flag.Name == "help" || !isSharedGlobalFlag(flag) || !isSharedGlobalOption(flag.Name) {
					return
				}
				if _, ok := discovered[flag.Name]; ok {
					return
				}
				discovered[flag.Name] = optionDoc{
					name:      flag.Name,
					token:     flagToken(flag),
					usage:     strings.TrimSpace(flag.Usage),
					required:  isFlagRequired(flag),
					inherited: false,
				}
			})
			walk(cmd)
		}
	}
	walk(root)

	options := make([]optionDoc, 0, len(discovered))
	for _, name := range sharedGlobalOptionOrder {
		option, ok := discovered[name]
		if !ok {
			continue
		}
		options = append(options, option)
	}
	return options
}

func isSharedGlobalOption(name string) bool {
	_, ok := sharedGlobalOptionIndex[name]
	return ok
}

func collectOptionDocs(flags *pflag.FlagSet, inherited bool) []optionDoc {
	options := make([]optionDoc, 0)
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Name == "help" {
			return
		}
		options = append(options, optionDoc{
			name:      flag.Name,
			token:     flagToken(flag),
			usage:     strings.TrimSpace(flag.Usage),
			required:  isFlagRequired(flag),
			inherited: inherited,
			shared:    isSharedGlobalFlag(flag),
		})
	})
	sort.Slice(options, func(i, j int) bool {
		return options[i].name < options[j].name
	})
	return options
}

func isSharedGlobalFlag(flag *pflag.Flag) bool {
	if flag == nil || flag.Annotations == nil {
		return false
	}
	values, ok := flag.Annotations[sharedGlobalFlagAnnotation]
	if !ok || len(values) == 0 {
		return false
	}
	return strings.EqualFold(values[0], "true") || values[0] == "1"
}

func flagToken(flag *pflag.Flag) string {
	token := "--" + flag.Name
	if flag.Shorthand != "" {
		token += "/-" + flag.Shorthand
	}
	return token
}

func isFlagRequired(flag *pflag.Flag) bool {
	values, ok := flag.Annotations[cobra.BashCompOneRequiredFlag]
	if !ok || len(values) == 0 {
		return false
	}
	return strings.EqualFold(values[0], "true") || values[0] == "1"
}

func optionLabels(option optionDoc) string {
	labels := make([]string, 0, 2)
	if option.required {
		labels = append(labels, "required")
	}
	if option.inherited {
		labels = append(labels, "global")
	}
	if len(labels) == 0 {
		return ""
	}
	return " [" + strings.Join(labels, ", ") + "]"
}
