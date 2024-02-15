package rust

import (
	"slices"
	"testing"
)

func mustEqual(t *testing.T, got, want []int, msg string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s, got %v want %v", msg, got, want)
	}
}

func TestDynamicSlice(t *testing.T) {
	var slice DynamicSlice[int]
	slice.Append(5, 6, 7)
	mustEqual(t, slice.Slice, []int{5, 6, 7}, "Append")
	slice.Insert(0, 42)
	mustEqual(t, slice.Slice, []int{42, 5, 6, 7}, "Insert")
	slice.Insert(2, 43)
	mustEqual(t, slice.Slice, []int{42, 5, 43, 6, 7}, "Insert")
	slice.Insert(5, 44)
	mustEqual(t, slice.Slice, []int{42, 5, 43, 6, 7, 44}, "Insert")
	slice.PopBack()
	mustEqual(t, slice.Slice, []int{42, 5, 43, 6, 7}, "PopBack")
	slice.PopFront()
	mustEqual(t, slice.Slice, []int{5, 43, 6, 7}, "PopFront")
	slice.Remove(1)
	mustEqual(t, slice.Slice, []int{5, 6, 7}, "Remove")
	slice.Set(1, 77)
	mustEqual(t, slice.Slice, []int{5, 77, 7}, "Set")
	slice.Truncate(2)
	mustEqual(t, slice.Slice, []int{5, 77}, "Truncate")
	slice.Reset([]int{2, 3, 5, 7, 11})
	mustEqual(t, slice.Slice, []int{2, 3, 5, 7, 11}, "Reset")
	slice.Append(13)
	mustEqual(t, slice.Slice, []int{2, 3, 5, 7, 11, 13}, "Append")
	slice.PushBack(17)
	mustEqual(t, slice.Slice, []int{2, 3, 5, 7, 11, 13, 17}, "PushBack")
	slice.PushFront(1)
	mustEqual(t, slice.Slice, []int{1, 2, 3, 5, 7, 11, 13, 17}, "PushFront")
	slice.Clear()
	mustEqual(t, slice.Slice, []int{}, "Clear")
}
