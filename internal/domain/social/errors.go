package social

import "fmt"

// ErrPostNotFound indicates the requested post does not exist.
var ErrPostNotFound = fmt.Errorf("post not found")

// ErrNotConfigured indicates the Instagram publisher is not set up.
var ErrNotConfigured = fmt.Errorf("instagram publishing not configured")

// ErrNotPublishable indicates the post is not in a publishable state.
var ErrNotPublishable = fmt.Errorf("cannot publish: caption not ready")
