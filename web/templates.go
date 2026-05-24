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
		return "bg-green-400/10"
	case "inactive":
		return "bg-yellow-400/10"
	case "error":
		return "bg-red-400/10"
	default:
		return "bg-gray-400/10"
	}
}

func statusText(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "text-green-400"
	case "inactive":
		return "text-yellow-400"
	case "error":
		return "text-red-400"
	default:
		return "text-gray-400"
	}
}

func statusBorder(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "border-green-400/20"
	case "inactive":
		return "border-yellow-400/20"
	case "error":
		return "border-red-400/20"
	default:
		return "border-gray-400/20"
	}
}

func statusDot(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "active":
		return "bg-green-400"
	case "inactive":
		return "bg-yellow-400"
	case "error":
		return "bg-red-400"
	default:
		return "bg-gray-400"
	}
}

// Deploy status helpers (success/failed/running/pending/rollback)
func deployBg(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "bg-green-400/10"
	case "failed":
		return "bg-red-400/10"
	case "running":
		return "bg-blue-400/10"
	case "pending":
		return "bg-yellow-400/10"
	case "rollback":
		return "bg-orange-400/10"
	default:
		return "bg-gray-400/10"
	}
}

func deployText(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "text-green-400"
	case "failed":
		return "text-red-400"
	case "running":
		return "text-blue-400"
	case "pending":
		return "text-yellow-400"
	case "rollback":
		return "text-orange-400"
	default:
		return "text-gray-400"
	}
}

func deployBorder(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "border-green-400/20"
	case "failed":
		return "border-red-400/20"
	case "running":
		return "border-blue-400/20"
	case "pending":
		return "border-yellow-400/20"
	case "rollback":
		return "border-orange-400/20"
	default:
		return "border-gray-400/20"
	}
}

func deployDot(status any) string {
	s := fmt.Sprintf("%s", status)
	switch s {
	case "success":
		return "bg-green-400"
	case "failed":
		return "bg-red-400"
	case "running":
		return "bg-blue-400"
	case "pending":
		return "bg-yellow-400"
	case "rollback":
		return "bg-orange-400"
	default:
		return "bg-gray-400"
	}
}

// Flash helpers for toast notifications (red/yellow/green/blue)
func flashBg(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "bg-red-400/10"
	case "yellow":
		return "bg-yellow-400/10"
	case "green":
		return "bg-green-400/10"
	case "blue":
		return "bg-blue-400/10"
	default:
		return "bg-gray-400/10"
	}
}

func flashText(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "text-red-400"
	case "yellow":
		return "text-yellow-400"
	case "green":
		return "text-green-400"
	case "blue":
		return "text-blue-400"
	default:
		return "text-gray-400"
	}
}

func flashBorder(color any) string {
	s := fmt.Sprintf("%s", color)
	switch s {
	case "red":
		return "border-red-400/20"
	case "yellow":
		return "border-yellow-400/20"
	case "green":
		return "border-green-400/20"
	case "blue":
		return "border-blue-400/20"
	default:
		return "border-gray-400/20"
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
