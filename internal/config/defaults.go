package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Work with YAML nodes so values, comments and aliases survive completion.
// The default document is the single source of fields to add, including future
// options. Explicit empty/null values are not silently replaced with defaults.
func completeDefaults(data []byte) ([]byte, bool, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false, fmt.Errorf("expected one YAML document")
	}
	if len(document.Content) == 0 {
		document = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, fmt.Errorf("configuration must be a YAML mapping")
	}
	// Decoding includes inherited merge keys, so defaults cannot override them.
	var existing map[string]any
	if err := root.Decode(&existing); err != nil {
		return nil, false, err
	}
	var defaults yaml.Node
	if err := yaml.Unmarshal([]byte(defaultYAML), &defaults); err != nil {
		return nil, false, err
	}
	fields := defaults.Content[0].Content
	changed := false
	for i := 0; i < len(fields); i += 2 {
		key := fields[i].Value
		if _, exists := existing[key]; exists {
			continue
		}
		root.Content = append(root.Content, fields[i], fields[i+1])
		changed = true
	}
	if !changed {
		return data, false, nil
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, false, err
	}
	if err := encoder.Close(); err != nil {
		return nil, false, err
	}
	return output.Bytes(), true, nil
}

// A same-directory temporary file keeps incomplete writes away from config.yaml
// and retains private permissions for proxy credentials.
func saveCompletedConfig(path string, original, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("complete %s: %w", path, err)
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("complete %s: %w", path, err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		// Another MeshVault process may have completed the same file already.
		if bytes.Equal(current, data) {
			return nil
		}
		return fmt.Errorf("%s changed while loading; retry to avoid overwriting edits", path)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("complete %s: %w", path, err)
	}
	return nil
}
