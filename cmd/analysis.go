package cmd

import (
	"os"
	"path/filepath"

	"github.com/RaniduNethma/vedoc/internal/models"
	"github.com/RaniduNethma/vedoc/internal/parser"
	"github.com/RaniduNethma/vedoc/internal/scanner"
)

func analyzeProject(root string) ([]models.Endpoint, error) {
	files, err := scanner.Discover(root)
	if err != nil {
		return nil, err
	}

	sourceFiles := make([]parser.SourceFile, 0, len(files))
	for _, filename := range files {
		source, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return nil, err
		}
		sourceFiles = append(sourceFiles, parser.SourceFile{
			Path:   filepath.ToSlash(relative),
			Source: source,
		})
	}

	return parser.ResolveExpressProject(sourceFiles)
}
