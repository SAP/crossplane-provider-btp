package options

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestWithBackoffDisablesPriorityQueue(t *testing.T) {
	co := CrossplaneOptions{}
	if got := co.ForControllerRuntimeWithBackoff().UsePriorityQueue; got == nil || *got != false {
		t.Errorf("CrossplaneOptions: UsePriorityQueue = %v, want %v", got, ptr.To(false))
	}
	uo := UpjetOptions{}
	if got := uo.ForControllerRuntimeWithBackoff().UsePriorityQueue; got == nil || *got != false {
		t.Errorf("UpjetOptions: UsePriorityQueue = %v, want %v", got, ptr.To(false))
	}
}
