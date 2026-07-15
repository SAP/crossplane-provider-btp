package v1alpha1

import (
	"testing"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestValidationCondition(t *testing.T) {
	type args struct {
		validationIssues []string
	}
	tests := []struct {
		name string
		args args
		want v1.Condition
	}{
		{
			name: "nil Validation Issue, NoValidationIssue",
			args: args{
				validationIssues: nil,
			},
			want: v1.Condition{
				Type:   SoftValidationCondition,
				Status: corev1.ConditionFalse,
				Reason: NoValidationIssues,
			},
		},
		{
			name: "One validation Issue, ValidationIssue",
			args: args{
				validationIssues: []string{"asd"},
			},
			want: v1.Condition{
				Type:    SoftValidationCondition,
				Status:  corev1.ConditionTrue,
				Reason:  HasValidationIssues,
				Message: "asd",
			},
		},
		{
			name: "Multiple validation Issue, ValidationIssue with joind message",
			args: args{
				validationIssues: []string{"foo", "bar"},
			},
			want: v1.Condition{
				Type:    SoftValidationCondition,
				Status:  corev1.ConditionTrue,
				Reason:  HasValidationIssues,
				Message: "foo\nbar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidationCondition(tt.args.validationIssues); !tt.want.Equal(got) {
				t.Errorf("ValidationCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}
