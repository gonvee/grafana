// Code generated - EDITING IS FUTILE. DO NOT EDIT.
//
// Generated by:
//     kinds/gen.go
// Using jennies:
//     GoTypesJenny
//     LatestJenny
//
// Run 'make gen-cue' from repository root to regenerate.

package folder

// Folder defines model for Folder.
type Folder struct {
	// Folder title (must be unique within the parent folder)
	Title string `json:"title"`

	// Folder UID
	Uid string `json:"uid"`
}
