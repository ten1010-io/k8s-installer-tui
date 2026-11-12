package fileio

import (
	"bytes"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// The files are edited rather than generated: a file the site already has is
// read as a yaml.Node tree, the values this tool manages are replaced in it, and
// everything else - comments, per node variables set by hand, keys added by a
// newer installer - is written back as it was.

// parseDoc returns the top-level mapping of a YAML document, an empty mapping
// for empty input.
func parseDoc(data []byte) (*yaml.Node, error) {
	var d yaml.Node
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	if len(d.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	top := d.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("top level is not a mapping")
	}
	// yaml.v3 hangs a comment block that follows the last key on the document and
	// not on the mapping, and so one that is set apart above the first key. The
	// document is dropped here, so they are moved onto the mapping, or the end of
	// a file that closes with commented out options - the python section of the
	// aipub example is one key and sixty lines of them - is lost on the first save
	top.HeadComment = joinComments(d.HeadComment, top.HeadComment)
	top.FootComment = joinComments(top.FootComment, d.FootComment)
	return top, nil
}

func joinComments(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n\n" + b
}

func encodeDoc(top *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{top}}); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// keyIndex is the index of the key node of key in the mapping m, or -1.
func keyIndex(m *yaml.Node, key string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func getKey(m *yaml.Node, key string) *yaml.Node {
	if i := keyIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// setKey replaces the value of key, keeping the comment on its line, or adds the
// key at the end of the mapping.
func setKey(m *yaml.Node, key string, val *yaml.Node) {
	if i := keyIndex(m, key); i >= 0 {
		old := m.Content[i+1]
		if val.LineComment == "" {
			val.LineComment = old.LineComment
		}
		if val.FootComment == "" {
			val.FootComment = old.FootComment
		}
		m.Content[i+1] = val
		return
	}
	m.Content = append(m.Content, strKey(key), val)
}

// deleteKey removes key and its value. The comment above the key goes with it;
// a comment above the next key stays.
func deleteKey(m *yaml.Node, key string) {
	if i := keyIndex(m, key); i >= 0 {
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
	}
}

// subMap returns the mapping under key, creating it (or replacing a non-mapping
// such as `hosts: {}` written as null) when needed.
func subMap(m *yaml.Node, key string) *yaml.Node {
	v := getKey(m, key)
	if v == nil || v.Kind != yaml.MappingNode {
		v = mapNode()
		setKey(m, key, v)
	}
	v.Style = 0 // a mapping that was {} gets entries written in block style
	return v
}

// keys lists the keys of a mapping in their order.
func keys(m *yaml.Node) []string {
	var out []string
	if m == nil || m.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, m.Content[i].Value)
	}
	return out
}

// --- node builders ---

func strKey(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// quoted is a string written in double quotes, as the installer's own files write them.
func quoted(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle}
}

func intNode(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
}

func boolNode(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
}

func nullNode() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"} }

// quotedOrNull is null for an empty string.
func quotedOrNull(s string) *yaml.Node {
	if s == "" {
		return nullNode()
	}
	return quoted(s)
}

func mapNode(kv ...interface{}) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Content = append(m.Content, strKey(kv[i].(string)), kv[i+1].(*yaml.Node))
	}
	return m
}

// emptyMap is written `{}`, as inventory.yml writes a group member with no variables.
func emptyMap() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
}

func quotedSeq(items []string) *yaml.Node {
	s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, it := range items {
		s.Content = append(s.Content, quoted(it))
	}
	return s
}

// flowSeq is a short list written on one line, `["node1", "node2"]`.
func flowSeq(items []string) *yaml.Node {
	s := quotedSeq(items)
	s.Style = yaml.FlowStyle
	return s
}

func seqNode(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}
