package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

type Renderer struct {
	layout *template.Template
	pages  map[string]*template.Template
}

func NewRenderer() (*Renderer, error) {
	funcMap := template.FuncMap{
		"statusBg":     statusBg,
		"statusText":   statusText,
		"statusBorder": statusBorder,
		"statusDot":    statusDot,
		"deployBg":     deployBg,
		"deployText":   deployText,
		"deployBorder": deployBorder,
		"deployDot":    deployDot,
		"flashBg":      flashBg,
		"flashText":    flashText,
		"flashBorder":  flashBorder,
		"timeAgo":      timeAgo,
		"truncateSHA":  truncateSHA,
		"dict":         dict,
		"formatSize": func(bytes int64) string {
			const unit = 1024
			if bytes < unit {
				return fmt.Sprintf("%d B", bytes)
			}
			div, exp := int64(unit), 0
			for n := bytes / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
		},
	}

	layout, err := template.New("layout.html").Funcs(funcMap).ParseFS(TemplateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}

	pages := make(map[string]*template.Template)

	// Parse each page template individually to avoid namespace collisions
	pageFiles, err := TemplateFS.ReadDir("templates/pages")
	if err != nil {
		return nil, fmt.Errorf("read pages dir: %w", err)
	}

	for _, f := range pageFiles {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		t, err := template.New(name).Funcs(funcMap).ParseFS(TemplateFS,
			"templates/pages/"+name,
			"templates/partials/*.html",
		)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		pages[name] = t
	}

	return &Renderer{layout: layout, pages: pages}, nil
}

type layoutData struct {
	Title   string
	User    *UserData
	Content template.HTML
}

type UserData struct {
	Username string
}

func (r *Renderer) Render(w http.ResponseWriter, pageName string, title string, user *UserData, pageData any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	t, ok := r.pages[pageName]
	if !ok {
		http.Error(w, "page not found: "+pageName, http.StatusInternalServerError)
		return
	}

	// Render page content into buffer
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, pageName, pageData); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Render layout with page content injected
	data := layoutData{
		Title:   title,
		User:    user,
		Content: template.HTML(buf.String()),
	}

	if err := r.layout.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "layout error", http.StatusInternalServerError)
	}
}

func (r *Renderer) RenderPartial(w http.ResponseWriter, pageName string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	t, ok := r.pages[pageName]
	if !ok {
		http.Error(w, "page not found: "+pageName, http.StatusInternalServerError)
		return
	}

	if err := t.ExecuteTemplate(w, pageName, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (r *Renderer) RenderStatus(w http.ResponseWriter, status int, pageName string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	t, ok := r.pages[pageName]
	if !ok {
		http.Error(w, "page not found: "+pageName, http.StatusInternalServerError)
		return
	}

	if err := t.ExecuteTemplate(w, pageName, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Status helpers for app status (active/inactive/error)
func statusBg(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "bg-lime-accent/10"
	case "inactive":
		return "bg-yellow-400/10"
	case "error":
		return "bg-pink-accent/10"
	default:
		return "bg-gray-400/10"
	}
}

func statusText(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "text-lime-accent"
	case "inactive":
		return "text-yellow-400"
	case "error":
		return "text-pink-accent"
	default:
		return "text-muted"
	}
}

func statusBorder(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "border-lime-accent/20"
	case "inactive":
		return "border-yellow-400/20"
	case "error":
		return "border-pink-accent/20"
	default:
		return "border-hairline"
	}
}

func statusDot(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "bg-lime-accent"
	case "inactive":
		return "bg-yellow-400"
	case "error":
		return "bg-pink-accent"
	default:
		return "bg-muted"
	}
}

// Deploy status helpers (success/failed/running/pending/rollback)
func deployBg(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "bg-lime-accent/10"
	case "failed":
		return "bg-pink-accent/10"
	case "running":
		return "bg-violet/10"
	case "pending":
		return "bg-yellow-400/10"
	case "rollback":
		return "bg-violet-mid/10"
	default:
		return "bg-gray-400/10"
	}
}

func deployText(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "text-lime-accent"
	case "failed":
		return "text-pink-accent"
	case "running":
		return "text-violet"
	case "pending":
		return "text-yellow-400"
	case "rollback":
		return "text-violet-mid"
	default:
		return "text-muted"
	}
}

func deployBorder(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "border-lime-accent/20"
	case "failed":
		return "border-pink-accent/20"
	case "running":
		return "border-violet/20"
	case "pending":
		return "border-yellow-400/20"
	case "rollback":
		return "border-violet-mid/20"
	default:
		return "border-hairline"
	}
}

func deployDot(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "bg-lime-accent"
	case "failed":
		return "bg-pink-accent"
	case "running":
		return "bg-violet"
	case "pending":
		return "bg-yellow-400"
	case "rollback":
		return "bg-violet-mid"
	default:
		return "bg-muted"
	}
}

// Flash helpers for toast notifications (red/yellow/green/blue)
func flashBg(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "bg-pink-accent/10"
	case "yellow":
		return "bg-yellow-400/10"
	case "green":
		return "bg-lime-accent/10"
	case "blue":
		return "bg-violet/10"
	default:
		return "bg-gray-400/10"
	}
}

func flashText(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "text-pink-accent"
	case "yellow":
		return "text-yellow-400"
	case "green":
		return "text-lime-accent"
	case "blue":
		return "text-violet"
	default:
		return "text-muted"
	}
}

func flashBorder(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "border-pink-accent/20"
	case "yellow":
		return "border-yellow-400/20"
	case "green":
		return "border-lime-accent/20"
	case "blue":
		return "border-violet/20"
	default:
		return "border-hairline"
	}
}

func timeAgo(t *time.Time) string {
	if t == nil {
		return "—"
	}
	d := time.Since(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	if sha == "" {
		return "—"
	}
	return sha
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}
	m := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		m[key] = values[i+1]
	}
	return m, nil
}
