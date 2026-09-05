package core

import (
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// stampPDFMetadata sets the Title, Author, Subject, and Keywords fields
// of the PDF at path, using pdfcpu. Best-effort — errors are returned so
// callers can log but shouldn't fail the whole job on this.
//
// author defaults to "Mac Transformer" when empty so Books.app has
// something to group by.
func stampPDFMetadata(path, title, author, subject string, keywords []string) error {
	if title == "" {
		title = filepath.Base(strings.TrimSuffix(path, filepath.Ext(path)))
	}
	if author == "" {
		author = "Mac Transformer"
	}
	props := map[string]string{
		"Title":  title,
		"Author": author,
	}
	if subject != "" {
		props["Subject"] = subject
	}
	if len(keywords) > 0 {
		props["Keywords"] = strings.Join(keywords, ", ")
	}
	// pdfcpu's AddProperties writes to Info dict; in-place edit is fine.
	return api.AddPropertiesFile(path, "", props, model.NewDefaultConfiguration())
}
