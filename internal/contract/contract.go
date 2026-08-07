package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/RaniduNethma/vedoc/internal/models"
)

const SnapshotVersion = 1

type Snapshot struct {
	Version   int               `json:"version"`
	Endpoints []models.Endpoint `json:"endpoints"`
}

type EndpointRef struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type DiffResult struct {
	Added                   []EndpointRef `json:"added,omitempty"`
	Removed                 []EndpointRef `json:"removed,omitempty"`
	IgnoredUnresolvedBefore int           `json:"ignoredUnresolvedBefore"`
	IgnoredUnresolvedAfter  int           `json:"ignoredUnresolvedAfter"`
	Breaking                bool          `json:"breaking"`
}

func BuildSnapshot(endpoints []models.Endpoint) Snapshot {
	copyEndpoints := make([]models.Endpoint, len(endpoints))
	for i, endpoint := range endpoints {
		copyEndpoints[i] = endpoint
		copyEndpoints[i].Description = ""
		copyEndpoints[i].Payload = ""
	}
	models.SortEndpoints(copyEndpoints)
	return Snapshot{Version: SnapshotVersion, Endpoints: copyEndpoints}
}

func WriteSnapshot(filename string, snapshot Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

func ReadSnapshot(filename string) (Snapshot, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if snapshot.Version != SnapshotVersion {
		return Snapshot{}, fmt.Errorf("unsupported contract snapshot version %d", snapshot.Version)
	}
	models.SortEndpoints(snapshot.Endpoints)
	return snapshot, nil
}

func Diff(before, after Snapshot) DiffResult {
	beforeSet, beforeUnresolved := resolvedSet(before.Endpoints)
	afterSet, afterUnresolved := resolvedSet(after.Endpoints)

	result := DiffResult{
		IgnoredUnresolvedBefore: beforeUnresolved,
		IgnoredUnresolvedAfter:  afterUnresolved,
	}

	for key, endpoint := range afterSet {
		if _, exists := beforeSet[key]; !exists {
			result.Added = append(result.Added, endpoint)
		}
	}
	for key, endpoint := range beforeSet {
		if _, exists := afterSet[key]; !exists {
			result.Removed = append(result.Removed, endpoint)
		}
	}

	sortEndpointRefs(result.Added)
	sortEndpointRefs(result.Removed)
	result.Breaking = len(result.Removed) > 0
	return result
}

func resolvedSet(endpoints []models.Endpoint) (map[string]EndpointRef, int) {
	set := make(map[string]EndpointRef)
	unresolved := 0
	for _, endpoint := range endpoints {
		if !endpoint.IsResolved() || endpoint.Path == "" {
			unresolved++
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(endpoint.Method))
		path := normalizeContractPath(endpoint.Path)
		ref := EndpointRef{Method: method, Path: path}
		set[method+"\x00"+path] = ref
	}
	return set, unresolved
}

func normalizeContractPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if len(value) > 1 {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func sortEndpointRefs(refs []EndpointRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Path != refs[j].Path {
			return refs[i].Path < refs[j].Path
		}
		return refs[i].Method < refs[j].Method
	})
}
