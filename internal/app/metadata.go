package app

// metadata.go holds the global metadata-discovery commands: priorities,
// labels, and the generic create-screen field/option discovery. Project-scoped
// discovery (components, versions, issue types, statuses) lives under
// `project` in project.go.

import (
	"context"
	"strings"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/spf13/cobra"
)

func newPriorityCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "priority",
		Short:   "Browse issue priorities",
		Aliases: []string{"priorities"},
	}
	cmd.AddCommand(newPriorityListCmd(s))
	return cmd
}

func newPriorityListCmd(s *appState) *cobra.Command {
	var (
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issue priorities",
		Long: "List the priorities issues can take — the valid values for\n" +
			"`issue create/edit --priority`. The list is instance-wide; on Jira\n" +
			"Cloud a priority scheme may narrow what a given project accepts.",
		Example: "  jira-cli priority list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[apiclient.Priority], error) {
				return client.ListPriorities(ctx, apiclient.ListOpts{Limit: limit, Cursor: c})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newLabelCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "label",
		Short:   "Browse issue labels",
		Aliases: []string{"labels"},
	}
	cmd.AddCommand(newLabelListCmd(s))
	return cmd
}

func newLabelListCmd(s *appState) *cobra.Command {
	var (
		limit  int
		all    bool
		cursor string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issue labels (Jira Cloud only)",
		Long: "List every label in use on the instance. Labels are free-form and\n" +
			"instance-wide, not project-scoped. Data Center has no REST endpoint\n" +
			"for this listing; there, discover labels from the issues that carry\n" +
			"them (e.g. `issue search --project X --field labels`).",
		Example: "  jira-cli label list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			fetch := func(c string) (apiclient.ListResult[string], error) {
				return client.ListLabels(ctx, apiclient.ListOpts{Limit: limit, Cursor: c})
			}
			items, info, err := collectPage(fetch, cursor, all)
			if err != nil {
				return err
			}
			return s.emitList(items, info)
		},
	}
	addListFlags(cmd, &limit, &all, &cursor)
	return cmd
}

func newFieldCmd(s *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "field",
		Short:   "Discover issue fields and their allowed values",
		Aliases: []string{"fields"},
	}
	cmd.AddCommand(newFieldListCmd(s), newFieldOptionsCmd(s))
	return cmd
}

func newFieldListCmd(s *appState) *cobra.Command {
	var (
		project string
		typ     string
	)
	cmd := &cobra.Command{
		Use:   "list --project <key> --type <issue-type>",
		Short: "List the fields on an issue type's create screen",
		Long: "List the fields available when creating an issue of one type in one\n" +
			"project: id, name, whether it is required, its schema, and how many\n" +
			"allowed values it has (options_count). Use `field options` to read a\n" +
			"constrained field's actual values. Custom fields appear here with\n" +
			"their customfield_* id.",
		Example: "  jira-cli field list --project ENG --type Bug\n" +
			"  jira-cli field list --project ENG --type Bug --fields items.id,items.name,items.required",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := requireProject(s, project)
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if typ == "" {
				return cerrors.New(cerrors.CategoryUsage, "FIELD_NO_TYPE",
					"pass --type with an issue type name or ID (fields differ per issue type)").
					WithNextSteps("jira-cli project issuetypes " + key)
			}
			it, err := resolveIssueType(ctx, client, key, typ)
			if err != nil {
				return err
			}
			fields, err := collectCreateFields(ctx, client, key, it.ID)
			if err != nil {
				return err
			}
			// The full option sets are `field options`' job; keep this listing
			// scannable but preserve the options_count signal.
			for i := range fields {
				fields[i].AllowedValues = nil
			}
			return s.emitList(fields, pageInfo{})
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project key (default from defaults.project)")
	f.StringVar(&typ, "type", "", "issue type name or ID (see `project issuetypes`)")
	return cmd
}

// fieldOptionsOutput is the result shape of `field options`: one field's
// allowed values, each annotated with the issue types it applies to.
type fieldOptionsOutput struct {
	Project string           `json:"project"`
	Field   fieldIdent       `json:"field"`
	Options []fieldOptionRow `json:"options"`
}

type fieldIdent struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Items string `json:"items,omitempty"`
}

type fieldOptionRow struct {
	ID         string   `json:"id,omitempty"`
	Value      string   `json:"value"`
	IssueTypes []string `json:"issue_types,omitempty"`
}

func newFieldOptionsCmd(s *appState) *cobra.Command {
	var (
		project string
		typ     string
	)
	cmd := &cobra.Command{
		Use:   "options <field> --project <key> [--type <issue-type>]",
		Short: "List a field's allowed values in a project",
		Long: "List the allowed values of a constrained issue field (components,\n" +
			"fix/affects versions, priority, or a select-list custom field) in one\n" +
			"project, from the create metadata. Jira scopes field options by\n" +
			"context — project and issue type — so with --type the values are read\n" +
			"for that one issue type; without it every creatable issue type is\n" +
			"scanned and each value lists the issue types it applies to.\n\n" +
			"<field> is a field id (components, priority, customfield_10010) or a\n" +
			"display name (\"Component/s\", \"Severity\"), matched case-insensitively.",
		Example: "  jira-cli field options components --project ENG\n" +
			"  jira-cli field options \"Severity\" --project ENG --type Bug",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := requireProject(s, project)
			if err != nil {
				return err
			}
			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			out, err := fieldOptions(ctx, client, key, typ, args[0])
			if err != nil {
				return err
			}
			return s.emit(out)
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project key (default from defaults.project)")
	f.StringVar(&typ, "type", "", "restrict to one issue type name or ID (default: scan all)")
	return cmd
}

// requireProject resolves the effective project key from a --project flag or
// the configured default.
func requireProject(s *appState, flag string) (string, error) {
	if flag != "" {
		return projectKeyArg(flag), nil
	}
	if p := s.cfg().Defaults.Project; p != "" {
		return p, nil
	}
	return "", cerrors.New(cerrors.CategoryUsage, "FIELD_NO_PROJECT",
		"a project is required: pass --project or set defaults.project").
		WithNextSteps("jira-cli project list")
}

// resolveIssueType maps an issue type selector (ID or name) onto one of the
// project's creatable issue types. Misses and ambiguity fail with the
// candidate list.
func resolveIssueType(ctx context.Context, client apiclient.Client, projectKey, selector string) (apiclient.IssueType, error) {
	types, err := collectProjectIssueTypes(ctx, client, projectKey)
	if err != nil {
		return apiclient.IssueType{}, err
	}
	var byName []apiclient.IssueType
	for _, t := range types {
		if t.ID == selector {
			return t, nil
		}
		if strings.EqualFold(t.Name, selector) {
			byName = append(byName, t)
		}
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	candidates := make([]string, 0, len(types))
	for _, t := range types {
		candidates = append(candidates, t.Name+" (id "+t.ID+")")
	}
	if len(byName) > 1 {
		return apiclient.IssueType{}, cerrors.Newf(cerrors.CategoryUsage, "ISSUETYPE_AMBIGUOUS",
			"%q matches %d issue types in %s", selector, len(byName), projectKey).
			WithHint("Pass the issue type ID instead.").
			WithNextSteps(candidates...)
	}
	return apiclient.IssueType{}, cerrors.Newf(cerrors.CategoryNotFound, "ISSUETYPE_NOT_FOUND",
		"no issue type in %s matches %q", projectKey, selector).
		WithNextSteps(candidates...)
}

func collectProjectIssueTypes(ctx context.Context, client apiclient.Client, projectKey string) ([]apiclient.IssueType, error) {
	return apiclient.CollectAll(func(cursor string) (apiclient.ListResult[apiclient.IssueType], error) {
		return client.ListProjectIssueTypes(ctx, apiclient.ProjectItemsOpts{
			ListOpts:   apiclient.ListOpts{Cursor: cursor},
			ProjectKey: projectKey,
		})
	}, 0)
}

func collectCreateFields(ctx context.Context, client apiclient.Client, projectKey, issueTypeID string) ([]apiclient.FieldMeta, error) {
	return apiclient.CollectAll(func(cursor string) (apiclient.ListResult[apiclient.FieldMeta], error) {
		return client.ListCreateFields(ctx, apiclient.CreateFieldsOpts{
			ListOpts:    apiclient.ListOpts{Cursor: cursor},
			ProjectKey:  projectKey,
			IssueTypeID: issueTypeID,
		})
	}, 0)
}

// fieldOptions gathers one field's allowed values across the requested issue
// type context(s) and annotates every value with the issue types offering it.
func fieldOptions(ctx context.Context, client apiclient.Client, projectKey, typeSelector, fieldSelector string) (*fieldOptionsOutput, error) {
	var types []apiclient.IssueType
	if typeSelector != "" {
		it, err := resolveIssueType(ctx, client, projectKey, typeSelector)
		if err != nil {
			return nil, err
		}
		types = []apiclient.IssueType{it}
	} else {
		all, err := collectProjectIssueTypes(ctx, client, projectKey)
		if err != nil {
			return nil, err
		}
		types = all
	}

	out := &fieldOptionsOutput{Project: projectKey}
	// index and order preserve first-seen option order across issue types.
	index := map[string]*fieldOptionRow{}
	var order []string
	// optionable remembers fields that do carry options, for the miss error.
	optionable := map[string]string{}
	found := false
	for _, it := range types {
		fields, err := collectCreateFields(ctx, client, projectKey, it.ID)
		if err != nil {
			return nil, err
		}
		for _, fm := range fields {
			if len(fm.AllowedValues) > 0 {
				optionable[fm.ID] = fm.Name
			}
			if !matchField(fm, fieldSelector) {
				continue
			}
			found = true
			if out.Field.ID == "" {
				out.Field = fieldIdent{ID: fm.ID, Name: fm.Name, Type: fm.Type, Items: fm.Items}
			}
			for _, v := range fm.AllowedValues {
				k := v.ID + "\x00" + v.Value
				row, ok := index[k]
				if !ok {
					row = &fieldOptionRow{ID: v.ID, Value: v.Value}
					index[k] = row
					order = append(order, k)
				}
				row.IssueTypes = append(row.IssueTypes, it.Name)
			}
		}
	}
	if !found {
		candidates := make([]string, 0, len(optionable))
		for id, name := range optionable {
			candidates = append(candidates, name+" ("+id+")")
		}
		return nil, cerrors.Newf(cerrors.CategoryNotFound, "FIELD_NOT_FOUND",
			"no create-screen field in %s matches %q", projectKey, fieldSelector).
			WithHint("Fields with allowed values in this project are listed below; " +
				"see the full set with `jira-cli field list --project " + projectKey + " --type <type>`.").
			WithNextSteps(candidates...)
	}
	for _, k := range order {
		out.Options = append(out.Options, *index[k])
	}
	return out, nil
}

// matchField reports whether a field matches a selector: the field id
// (case-insensitive) or the display name (case-insensitive).
func matchField(fm apiclient.FieldMeta, selector string) bool {
	return strings.EqualFold(fm.ID, selector) || strings.EqualFold(fm.Name, selector)
}
