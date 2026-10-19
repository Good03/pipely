package pipely

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rivo/tview"
)

func (u *terminalUI) runForm(p Pipeline, opts RunOptions) {
	u.loadRunForm(p, opts, nil)
}

func (u *terminalUI) loadRunForm(p Pipeline, opts RunOptions, branches []string) {
	opts.Branch = strings.TrimPrefix(opts.Branch, "refs/heads/")
	f := tview.NewForm()
	initializing := true
	if len(branches) > 0 && u.editingGroupName == "" {
		index := 0
		for i, b := range branches {
			if b == opts.Branch {
				index = i
			}
		}
		f.AddDropDown("Branch", branches, index, func(s string, _ int) {
			if initializing || s == opts.Branch {
				return
			}
			next := opts
			next.Branch = s
			u.loadRunForm(p, next, branches)
		})
	}
	f.AddTextView("Options", "Loading pipeline parameters…", 60, 1, false, false)
	f.AddButton("Retry loading options", func() { u.loadRunForm(p, opts, branches) })
	initializing = false
	u.show("run-form", "Configure run · "+tview.Escape(p.Name), f)
	cl := u.client
	u.asyncRead(func(ctx context.Context) (func(), error) {
		m, e := cl.Metadata(ctx, p.Id, opts.Branch)
		if e != nil {
			if errors.Is(e, context.DeadlineExceeded) {
				e = fmt.Errorf("Loading pipeline options timed out. Retry loading options or select another branch.\n\nDetails: %w", e)
			}
			if len(m.Branches) == 0 {
				m.Branches = branches
			}
			if m.Branch == "" {
				m.Branch = opts.Branch
			}
			m.Problem = e.Error()
			return func() {
				u.renderRunForm(p, opts, m)
				u.lastError = e.Error()
				u.status(strings.SplitN(e.Error(), "\n", 2)[0])
			}, nil
		}
		return func() { u.renderRunForm(p, opts, m) }, e
	})
}
func (u *terminalUI) renderRunForm(p Pipeline, opts RunOptions, m Metadata) {
	form := tview.NewForm()
	params := map[string]any{}
	for k, v := range opts.Parameters {
		params[k] = v
	}
	branch := m.Branch
	selected := 0
	for i, b := range m.Branches {
		if b == branch {
			selected = i
		}
	}
	initializing := true
	if u.editingGroupName != "" {
		label := branch
		if label == "" {
			label = "Azure default branch"
		}
		form.AddTextView("Branch", label, 60, 1, false, false)
	} else if len(m.Branches) > 0 {
		form.AddDropDown("Branch", m.Branches, selected, func(s string, _ int) {
			if initializing || s == branch {
				return
			}
			next := opts
			next.Branch = s
			next.Parameters = params
			u.loadRunForm(p, next, m.Branches)
		})
	} else {
		form.AddTextView("Branch", "Could not load branch choices", 60, 1, false, false)
	}
	if m.Problem != "" {
		fMessage := strings.SplitN(m.Problem, "\n", 2)[0]
		form.AddTextView("Unavailable", fMessage, 60, 3, false, true)
		form.AddButton("Error details", func() { u.info("Pipeline options unavailable · F2 copy", m.Problem) })
		form.AddButton("Retry", func() { next := opts; next.Branch = branch; u.loadRunForm(p, next, m.Branches) })
		initializing = false
		u.show("run-form", "Configure run · "+tview.Escape(p.Name), form)
		u.lastError = m.Problem
		u.status(fMessage)
		return
	}
	invalid := map[string]string{}
	for _, definition := range m.Parameters {
		def := definition
		name := def.Name
		label := def.DisplayName
		if label == "" {
			label = name
		}
		value, exists := params[name]
		if !exists {
			value = def.Default
		}
		delete(params, name)
		if def.Type == "stringList" && len(def.Values) > 0 {
			chosen := map[string]bool{}
			switch old := value.(type) {
			case []any:
				for _, v := range old {
					chosen[fmt.Sprint(v)] = true
				}
			case []string:
				for _, v := range old {
					chosen[v] = true
				}
			}
			update := func() {
				values := []any{}
				for _, v := range def.Values {
					if chosen[fmt.Sprint(v)] {
						values = append(values, v)
					}
				}
				params[name] = values
			}
			update()
			for _, v := range def.Values {
				v := v
				key := fmt.Sprint(v)
				form.AddCheckbox(label+" / "+key, chosen[key], func(on bool) { chosen[key] = on; update() })
			}
			continue
		}
		if len(def.Values) > 0 {
			options := []string{}
			index := 0
			for i, v := range def.Values {
				options = append(options, fmt.Sprint(v))
				if fmt.Sprint(v) == fmt.Sprint(value) {
					index = i
				}
			}
			params[name] = def.Values[index]
			form.AddDropDown(label, options, index, func(_ string, i int) { params[name] = def.Values[i] })
			continue
		}
		switch def.Type {
		case "boolean":
			on, _ := value.(bool)
			params[name] = on
			form.AddCheckbox(label, on, func(on bool) { params[name] = on })
		case "string", "number", "":
			text := ""
			if value != nil {
				text = fmt.Sprint(value)
			}
			apply := func(s string) {
				delete(invalid, name)
				if def.Type == "number" {
					n, e := strconv.ParseFloat(s, 64)
					if e != nil {
						invalid[name] = label + " must be a number"
						return
					}
					params[name] = n
				} else {
					params[name] = s
					if value == nil && s == "" {
						invalid[name] = "Enter a value for " + label
					}
				}
			}
			apply(text)
			form.AddInputField(label, text, 50, nil, apply)
		default:
			if value == nil {
				invalid[name] = fmt.Sprintf("%s is a structured parameter without a default; define a default or allowed values in the pipeline", label)
				form.AddTextView(label, "No pipeline default available", 50, 2, false, false)
			} else {
				params[name] = value
				form.AddTextView(label, "Pipeline default", 50, 1, false, false)
			}
		}
	}
	form.AddButton("Select stages", func() {
		for _, def := range m.Parameters {
			if message := invalid[def.Name]; message != "" {
				u.status(message)
				return
			}
		}
		if branch != m.Branch {
			u.status("Branch options are still loading; retry loading before continuing")
			return
		}
		parameters := map[string]any{}
		for _, def := range m.Parameters {
			parameters[def.Name] = params[def.Name]
		}
		options := RunOptions{PipelineID: p.Id, Branch: branch, Parameters: parameters}
		cl := u.client
		u.asyncRead(func(ctx context.Context) (func(), error) {
			plan, e := cl.Prepare(ctx, options)
			return func() { u.stageScreen(p, plan, opts.Stages) }, e
		})
	})
	form.AddButton("Reload options", func() { next := opts; next.Branch = branch; next.Parameters = params; u.runForm(p, next) })
	initializing = false
	u.show("run-form", "Configure run · "+tview.Escape(p.Name), form)
	if u.editingGroupName != "" {
		u.status("Editing group " + u.editingGroupName)
	} else {
		u.status("Configure pipeline")
	}
}
