// SPDX-License-Identifier: MIT

package mybottest

// SetOpened makes every event stream of s run f after it has answered, before it sends anything.
func SetOpened(s *Server, f func()) { s.opened = f }
