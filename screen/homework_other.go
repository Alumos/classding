//go:build !windows

package main

type previewHomeworkWindows struct{}

func newHomeworkWindows(_ func(string, string)) (homeworkWindowManager, error) {
	return previewHomeworkWindows{}, nil
}
func (previewHomeworkWindows) update(_ string, _ []HomeworkCard, _ bool) error { return nil }
func (previewHomeworkWindows) close()                                          {}
