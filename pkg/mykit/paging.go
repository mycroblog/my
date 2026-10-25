// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"iter"
)

// PageFunc fetches the page of a collection that cursor names (nil for the first page) and returns its items
// and its next_cursor (nil or empty on the last page).
type PageFunc[T any] func(ctx context.Context, cursor *Cursor) (items []T, next *Cursor, err error)

// All iterates the items of a collection with keyset pagination, fetching the pages one by one as the loop
// needs them by following next_cursor. It stops after the last page, at the first error (yielded with the zero
// T), or when the loop breaks:
//
//	for st, err := range mykit.All(ctx, func(ctx context.Context, c *mykit.Cursor) ([]mykit.Status, *mykit.Cursor, error) {
//		r, err := api.GetHomeTimelineWithResponse(ctx, &mykit.GetHomeTimelineParams{Cursor: c})
//		if err != nil {
//			return nil, nil, err
//		}
//		return r.JSON200.Items, r.JSON200.NextCursor, nil
//	}) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(st.Id)
//	}
//
// A timeline has no end: break out of the loop when you have seen enough.
func All[T any](ctx context.Context, fetch PageFunc[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var cursor *Cursor
		for {
			items, next, err := fetch(ctx, cursor)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if next == nil || *next == "" {
				return
			}
			cursor = next
		}
	}
}
