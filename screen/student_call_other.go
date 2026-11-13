//go:build !windows

package main

type noopStudentCallWindow struct{}

func newStudentCallWindow(_ func(int)) (studentCallWindow, error) {
	return noopStudentCallWindow{}, nil
}
func (noopStudentCallWindow) update(studentCallView) error { return nil }
func (noopStudentCallWindow) close()                       {}
