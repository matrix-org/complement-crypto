package rust

import (
	"fmt"

	"golang.org/x/exp/slices"
)

type DynamicSlice[T any] struct {
	Slice []T
}

// Insert the item, shifting anything in this place up by one.
func (s *DynamicSlice[T]) Insert(i int, val T) {
	s.Slice = slices.Insert(s.Slice, i, val)
}

// Remove the item, shifting anything above this down by one.
func (s *DynamicSlice[T]) Remove(i int) {
	s.Slice = slices.Delete(s.Slice, i, i+1)
}

// Append items to the end of the slice.
func (s *DynamicSlice[T]) Append(vals ...T) {
	s.Slice = append(s.Slice, vals...)
}

// Set the item at i to val, does not shift anything.
func (s *DynamicSlice[T]) Set(i int, val T) {
	if i > len(s.Slice) {
		panic(fmt.Sprintf("DynamicSlice.Set[%d, %+v] out of bounds of size %d", i, val, len(s.Slice)))
	} else if i == len(s.Slice) {
		s.Slice = append(s.Slice, val)
	} else {
		s.Slice[i] = val
	}
}

func (s *DynamicSlice[T]) PushBack(val T) {
	s.Append(val)
}

func (s *DynamicSlice[T]) PushFront(val T) {
	s.Insert(0, val)
}

func (s *DynamicSlice[T]) PopBack() {
	s.Remove(len(s.Slice) - 1)
}

func (s *DynamicSlice[T]) PopFront() {
	s.Remove(0)
}

func (s *DynamicSlice[T]) Clear() {
	s.Slice = s.Slice[:0]
}

func (s *DynamicSlice[T]) Reset(vals []T) {
	s.Clear()
	s.Append(vals...)
}

func (s *DynamicSlice[T]) Truncate(length int) {
	s.Slice = s.Slice[0:length]
}
