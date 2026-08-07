package models

import "sort"

type ResolutionState string

const (
	ResolutionResolved   ResolutionState = "resolved"
	ResolutionUnresolved ResolutionState = "unresolved"
)

type SourceLocation struct {
	File    string `json:"file"`
	Line    uint32 `json:"line"`
	Column  uint32 `json:"column"`
	Kind    string `json:"kind"`
	Snippet string `json:"snippet,omitempty"`
}

type Endpoint struct {
	Method           string           `json:"method"`
	Path             string           `json:"path,omitempty"`
	LocalPath        string           `json:"localPath,omitempty"`
	Resolution       ResolutionState  `json:"resolution"`
	UnresolvedReason string           `json:"unresolvedReason,omitempty"`
	Source           []SourceLocation `json:"source,omitempty"`
	Middleware       []string         `json:"middleware,omitempty"`
	CodeSnippet      string           `json:"codeSnippet,omitempty"`
	Description      string           `json:"description,omitempty"`
	Payload          string           `json:"payload,omitempty"`
}

func (e Endpoint) IsResolved() bool {
	return e.Resolution == "" || e.Resolution == ResolutionResolved
}

func ResolvedEndpoints(endpoints []Endpoint) []Endpoint {
	resolved := make([]Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.IsResolved() && endpoint.Path != "" {
			resolved = append(resolved, endpoint)
		}
	}
	SortEndpoints(resolved)
	return resolved
}

func SortEndpoints(endpoints []Endpoint) {
	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].Resolution != endpoints[j].Resolution {
			return endpoints[i].Resolution < endpoints[j].Resolution
		}
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		if endpoints[i].LocalPath != endpoints[j].LocalPath {
			return endpoints[i].LocalPath < endpoints[j].LocalPath
		}
		if endpoints[i].Method != endpoints[j].Method {
			return endpoints[i].Method < endpoints[j].Method
		}
		if len(endpoints[i].Source) == 0 || len(endpoints[j].Source) == 0 {
			return len(endpoints[i].Source) < len(endpoints[j].Source)
		}
		if endpoints[i].Source[0].File != endpoints[j].Source[0].File {
			return endpoints[i].Source[0].File < endpoints[j].Source[0].File
		}
		return endpoints[i].Source[0].Line < endpoints[j].Source[0].Line
	})
}
