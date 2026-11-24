package grouping

import (
	"slices"
	"strings"

	"github.com/devforth/exceptic/internal/sentry"
)

// exceptionGroupComponents groups an exception group (Python's ExceptionGroup,
// JavaScript's AggregateError), which SDKs send flat with exception_id and
// parent_id, by its tree: each group by its distinct children in any order,
// so a group of three ValueErrors is the issue of one of four, and a
// top-level group of one distinct child is that child's issue. False when
// the list holds no group: the chain then counts as sent.
func exceptionGroupComponents(exceptions []sentry.Exception, javascript bool) ([]string, bool) {
	tree := newExceptionTree(exceptions, javascript)

	if tree == nil {
		return nil, false
	}

	var components []string

	for index := range exceptions {
		if tree.parent[index] >= 0 {
			continue
		}

		root := index

		if children := tree.distinctChildren(root); tree.isGroup(root) && len(children) == 1 {
			root = children[0]
		}

		components = append(components, tree.key(root)...)
	}

	// Ids that only loop leave no root: the chain counts as sent.
	return components, len(components) > 0
}

// exceptionTree links the flat list by exception_id and parent_id.
type exceptionTree struct {
	exceptions []sentry.Exception
	javascript bool
	parent     []int // index of the parent, -1 for none
	children   [][]int
	keys       map[int][]string
	visiting   map[int]bool
}

// newExceptionTree is nil when the list holds no exception group.
func newExceptionTree(exceptions []sentry.Exception, javascript bool) *exceptionTree {
	byID := map[int]int{}
	hasGroup := false

	for index, exception := range exceptions {
		if mechanism := exception.Mechanism; mechanism != nil {
			hasGroup = hasGroup || mechanism.IsExceptionGroup

			if mechanism.ExceptionID != nil {
				byID[*mechanism.ExceptionID] = index
			}
		}
	}

	if !hasGroup {
		return nil
	}

	tree := &exceptionTree{
		exceptions: exceptions,
		javascript: javascript,
		parent:     make([]int, len(exceptions)),
		children:   make([][]int, len(exceptions)),
		keys:       map[int][]string{},
		visiting:   map[int]bool{},
	}

	for index, exception := range exceptions {
		tree.parent[index] = -1

		if mechanism := exception.Mechanism; mechanism != nil && mechanism.ParentID != nil {
			if parent, ok := byID[*mechanism.ParentID]; ok && parent != index {
				tree.parent[index] = parent
				tree.children[parent] = append(tree.children[parent], index)
			}
		}
	}

	return tree
}

func (tree *exceptionTree) isGroup(index int) bool {
	mechanism := tree.exceptions[index].Mechanism

	return mechanism != nil && mechanism.IsExceptionGroup
}

// distinctChildren are the children of an exception, one of each key, in
// the order of their keys; a group's children may come in any order.
func (tree *exceptionTree) distinctChildren(index int) []int {
	var children []int

	seen := map[string]bool{}

	for _, child := range tree.children[index] {
		key := strings.Join(tree.key(child), "\n")

		if !seen[key] {
			seen[key] = true

			children = append(children, child)
		}
	}

	if tree.isGroup(index) {
		slices.SortFunc(children, func(first, second int) int {
			return slices.Compare(tree.key(first), tree.key(second))
		})
	}

	return children
}

// key is what an exception and its descendants group by. An exception met
// again through a loop of the SDK's ids adds nothing.
func (tree *exceptionTree) key(index int) []string {
	if key, ok := tree.keys[index]; ok {
		return key
	}

	if tree.visiting[index] {
		return nil
	}

	tree.visiting[index] = true
	key := exceptionComponents(tree.exceptions[index], tree.javascript)

	for _, child := range tree.distinctChildren(index) {
		key = append(key, "{")
		key = append(key, tree.key(child)...)
		key = append(key, "}")
	}

	tree.keys[index] = key

	return key
}
