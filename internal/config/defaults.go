package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Work with YAML nodes so values and comments survive completion and sorting.
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
	for i := 0; i < len(fields); i += 2 {
		key := fields[i].Value
		if _, exists := existing[key]; exists {
			continue
		}
		root.Content = append(root.Content, fields[i], fields[i+1])
	}
	// Materialize version even when it was inherited through a YAML merge.
	hasVersion := false
	for i := 0; i < len(root.Content); i += 2 {
		hasVersion = hasVersion || root.Content[i].Value == "version"
	}
	if !hasVersion {
		value := &yaml.Node{}
		if err := value.Encode(existing["version"]); err != nil {
			return nil, false, err
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"}, value)
	}
	// Expand aliases before reordering: YAML requires an anchor to precede its
	// aliases, but alphabetic key ordering could otherwise reverse that order.
	expandConfigAliases(root)
	type field struct{ key, value *yaml.Node }
	ordered := make([]field, 0, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		ordered = append(ordered, field{root.Content[i], root.Content[i+1]})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].key.Value == "version" {
			return ordered[j].key.Value != "version"
		}
		if ordered[j].key.Value == "version" {
			return false
		}
		return ordered[i].key.Value < ordered[j].key.Value
	})
	root.Style = 0 // Emit a block mapping, even if the input used flow style.
	root.Content = root.Content[:0]
	for _, field := range ordered {
		if field.key.Value == "version" {
			field.key.Style = 0
		}
		root.Content = append(root.Content, field.key, field.value)
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
	lines := strings.Split(output.String(), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "version:") {
			continue
		}
		end := i + 1
		for end < len(lines) && lines[end] == "" {
			end++
		}
		lines = append(append(lines[:i+1:i+1], ""), lines[end:]...)
		break
	}
	formatted := []byte(strings.Join(lines, "\n"))
	return formatted, !bytes.Equal(data, formatted), nil
}

func expandConfigAliases(node *yaml.Node) {
	if node.Kind == yaml.AliasNode {
		head, line, foot := node.HeadComment, node.LineComment, node.FootComment
		replacement := *node.Alias
		replacement.Content = append([]*yaml.Node(nil), replacement.Content...)
		*node = replacement
		if head != "" {
			node.HeadComment = head
		}
		if line != "" {
			node.LineComment = line
		}
		if foot != "" {
			node.FootComment = foot
		}
	}
	node.Anchor = ""
	for i, child := range node.Content {
		copy := *child
		copy.Content = append([]*yaml.Node(nil), child.Content...)
		node.Content[i] = &copy
		expandConfigAliases(&copy)
	}
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
