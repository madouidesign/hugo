// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package tpl contains template functions and related types.
package tpl

import (
	"context"
	"io"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	bp "github.com/gohugoio/hugo/bufferpool"
	"github.com/gohugoio/hugo/output/layouts"

	"github.com/gohugoio/hugo/output"

	htmltemplate "github.com/gohugoio/hugo/tpl/internal/go_templates/htmltemplate"
	texttemplate "github.com/gohugoio/hugo/tpl/internal/go_templates/texttemplate"
)

// TemplateManager manages the collection of templates.
type TemplateManager interface {
	TemplateHandler
	TemplateFuncGetter
	AddTemplate(name, tpl string) error
	MarkReady() error
}

// TemplateVariants describes the possible variants of a template.
// All of these may be empty.
type TemplateVariants struct {
	Language     string
	OutputFormat output.Format
}

// TemplateFinder finds templates.
type TemplateFinder interface {
	TemplateLookup
	TemplateLookupVariant
}

// UnusedTemplatesProvider lists unused templates if the build is configured to track those.
type UnusedTemplatesProvider interface {
	UnusedTemplates() []FileInfo
}

// TemplateHandlers holds the templates needed by Hugo.
type TemplateHandlers struct {
	Tmpl    TemplateHandler
	TxtTmpl TemplateParseFinder
}

// TemplateHandler finds and executes templates.
type TemplateHandler interface {
	TemplateFinder
	ExecuteWithContext(ctx context.Context, t Template, wr io.Writer, data any) error
	LookupLayout(d layouts.LayoutDescriptor, f output.Format) (Template, bool, error)
	HasTemplate(name string) bool
}

type TemplateLookup interface {
	Lookup(name string) (Template, bool)
}

type TemplateLookupVariant interface {
	// TODO(bep) this currently only works for shortcodes.
	// We may unify and expand this variant pattern to the
	// other templates, but we need this now for the shortcodes to
	// quickly determine if a shortcode has a template for a given
	// output format.
	// It returns the template, if it was found or not and if there are
	// alternative representations (output format, language).
	// We are currently only interested in output formats, so we should improve
	// this for speed.
	LookupVariant(name string, variants TemplateVariants) (Template, bool, bool)
	LookupVariants(name string) []Template
}

// Template is the common interface between text/template and html/template.
type Template interface {
	Name() string
	Prepare() (*texttemplate.Template, error)
}

// TemplateParser is used to parse ad-hoc templates, e.g. in the Resource chain.
type TemplateParser interface {
	Parse(name, tpl string) (Template, error)
}

// TemplateParseFinder provides both parsing and finding.
type TemplateParseFinder interface {
	TemplateParser
	TemplateFinder
}

// TemplateDebugger prints some debug info to stdout.
type TemplateDebugger interface {
	Debug()
}

// templateInfo wraps a Template with some additional information.
type templateInfo struct {
	Template
	Info
}

// templateInfo wraps a Template with some additional information.
type templateInfoManager struct {
	Template
	InfoManager
}

// TemplatesProvider as implemented by deps.Deps.
type TemplatesProvider interface {
	Tmpl() TemplateHandler
	TextTmpl() TemplateParseFinder
}

// WithInfo wraps the info in a template.
func WithInfo(templ Template, info Info) Template {
	if manager, ok := info.(InfoManager); ok {
		return &templateInfoManager{
			Template:    templ,
			InfoManager: manager,
		}
	}

	return &templateInfo{
		Template: templ,
		Info:     info,
	}
}

var baseOfRe = regexp.MustCompile("template: (.*?):")

func extractBaseOf(err string) string {
	m := baseOfRe.FindStringSubmatch(err)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// TemplateFuncGetter allows to find a template func by name.
type TemplateFuncGetter interface {
	GetFunc(name string) (reflect.Value, bool)
}

// GetPageFromContext returns the top level Page.
func GetPageFromContext(ctx context.Context) any {
	return ctx.Value(texttemplate.PageContextKey)
}

// SetPageInContext sets the top level Page.
func SetPageInContext(ctx context.Context, p page) context.Context {
	return context.WithValue(ctx, texttemplate.PageContextKey, p)
}

type page interface {
	IsNode() bool
}

func GetHasLockFromContext(ctx context.Context) bool {
	if v := ctx.Value(texttemplate.HasLockContextKey); v != nil {
		return v.(bool)
	}
	return false
}

func SetHasLockInContext(ctx context.Context, hasLock bool) context.Context {
	return context.WithValue(ctx, texttemplate.HasLockContextKey, hasLock)
}

const hugoNewLinePlaceholder = "___hugonl_"

var (
	stripHTMLReplacerPre = strings.NewReplacer("\n", " ", "</p>", hugoNewLinePlaceholder, "<br>", hugoNewLinePlaceholder, "<br />", hugoNewLinePlaceholder)
	whitespaceRe         = regexp.MustCompile(`\s+`)
)

// StripHTML strips out all HTML tags in s.
func StripHTML(s string) string {
	// Shortcut strings with no tags in them
	if !strings.ContainsAny(s, "<>") {
		return s
	}

	pre := stripHTMLReplacerPre.Replace(s)
	preReplaced := pre != s

	s = htmltemplate.StripTags(pre)

	if preReplaced {
		s = strings.ReplaceAll(s, hugoNewLinePlaceholder, "\n")
	}

	var wasSpace bool
	b := bp.GetBuffer()
	defer bp.PutBuffer(b)
	for _, r := range s {
		isSpace := unicode.IsSpace(r)
		if !(isSpace && wasSpace) {
			b.WriteRune(r)
		}
		wasSpace = isSpace
	}

	if b.Len() > 0 {
		s = b.String()
	}

	return s
}
