package servicebindingclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sap/crossplane-provider-btp/apis/account/v1alpha1"
)

var (
	errMockInstanceDelete = errors.New("mock instance delete error")
)

func TestSBKeyRotator_RetireBinding(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-2 * time.Hour)
	futureTime := now.Add(2 * time.Hour)

	tests := []struct {
		name     string
		cr       *v1alpha1.ServiceBinding
		want     bool
		wantKeys int
	}{
		{
			name: "ForceRotation_NewRetirement",
			cr: &v1alpha1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ForceRotationKey: "true",
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:   "current-id",
						Name: "current-name",
					},
				},
			},
			want:     true,
			wantKeys: 1,
		},
		{
			name: "ForceRotation_AlreadyRetired",
			cr: &v1alpha1.ServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						ForceRotationKey: "true",
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:   "current-id",
						Name: "current-name",
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "current-id",
								Name:        "current-name",
								RetiredDate: metav1.Time{Time: time.Now().Add(-time.Hour)},
							},
						},
					},
				},
			},
			want:     true,
			wantKeys: 0, // No new keys should be added since it's already retired
		},
		{
			name: "RotationDue_NewRetirement",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							Frequency: &metav1.Duration{Duration: time.Hour},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:          "current-id",
						Name:        "current-name",
						CreatedDate: &metav1.Time{Time: pastTime},
					},
				},
			},
			want:     true,
			wantKeys: 1,
		},
		{
			name: "RotationNotDue",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							Frequency: &metav1.Duration{Duration: time.Hour * 4},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:          "current-id",
						Name:        "current-name",
						CreatedDate: &metav1.Time{Time: futureTime},
					},
				},
			},
			want:     false,
			wantKeys: 0,
		},
		{
			name: "NoRotationConfig",
			cr: &v1alpha1.ServiceBinding{
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:   "current-id",
						Name: "current-name",
					},
				},
			},
			want:     false,
			wantKeys: 0,
		},
		{
			name: "InvalidCreatedDate",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							Frequency: &metav1.Duration{Duration: time.Hour},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:          "current-id",
						Name:        "current-name",
						CreatedDate: nil, // invalid date case
					},
				},
			},
			want:     false,
			wantKeys: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSBKeyRotator(nil)
			originalRetiredKeys := len(tt.cr.Status.AtProvider.RetiredKeys)

			got := r.RetireBinding(tt.cr)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantKeys, len(tt.cr.Status.AtProvider.RetiredKeys)-originalRetiredKeys)

			if got && tt.wantKeys > 0 {
				if tt.name == "ForceRotation_NewRetirement" || tt.name == "RotationDue_NewRetirement" {
					assert.Nil(t, tt.cr.Status.AtProvider.CreatedDate)
				}

				retiredKey := tt.cr.Status.AtProvider.RetiredKeys[len(tt.cr.Status.AtProvider.RetiredKeys)-1]
				assert.Equal(t, "current-id", retiredKey.ID)
				assert.Equal(t, "current-name", retiredKey.Name)
			}
		})
	}
}

func TestSBKeyRotator_HasExpiredKeys(t *testing.T) {
	now := time.Now()
	expiredTime := now.Add(-2 * time.Hour)
	validTime := now.Add(-30 * time.Minute)

	tests := []struct {
		name string
		cr   *v1alpha1.ServiceBinding
		want bool
	}{
		{
			name: "HasExpiredKeys",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "expired-key",
								Name:        "expired-name",
								CreatedDate: metav1.Time{Time: expiredTime},
								RetiredDate: metav1.Time{Time: expiredTime},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "NoExpiredKeys",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "valid-key",
								Name:        "valid-name",
								CreatedDate: metav1.Time{Time: validTime},
								RetiredDate: metav1.Time{Time: time.Now()},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "NoRetiredKeys",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{},
				},
			},
			want: false,
		},
		{
			name: "NoTTLConfig",
			cr: &v1alpha1.ServiceBinding{
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "some-key",
								Name:        "some-name",
								CreatedDate: metav1.Time{Time: expiredTime},
								RetiredDate: metav1.Time{Time: expiredTime},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "InvalidCreatedDate",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "invalid-key",
								Name:        "invalid-name",
								CreatedDate: metav1.Time{}, // invalid date case
								RetiredDate: metav1.Time{}, // invalid date case
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSBKeyRotator(nil)
			got := r.HasExpiredKeys(tt.cr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSBKeyRotator_DeleteExpiredKeys(t *testing.T) {
	now := time.Now()
	expiredTime := now.Add(-2 * time.Hour)
	validTime := now.Add(-30 * time.Minute)

	tests := []struct {
		name                string
		cr                  *v1alpha1.ServiceBinding
		mockDeleter         InstanceDeleter
		wantNewKeysCount    int
		wantErr             bool
		wantDeleteCallCount int
	}{
		{
			name: "DeleteExpiredKeysSuccessfully",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "expired-key",
								Name:        "expired-name",
								CreatedDate: metav1.Time{Time: expiredTime},
								RetiredDate: metav1.Time{Time: expiredTime},
							},
							{
								ID:          "valid-key",
								Name:        "valid-name",
								CreatedDate: metav1.Time{Time: validTime},
								RetiredDate: metav1.Time{Time: time.Now()},
							},
						},
					},
				},
			},
			mockDeleter:         &MockInstanceDeleter{},
			wantNewKeysCount:    1,
			wantErr:             false,
			wantDeleteCallCount: 1,
		},
		{
			name: "DeleteExpiredKeysWithError",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "expired-key",
								Name:        "expired-name",
								CreatedDate: metav1.Time{Time: expiredTime},
								RetiredDate: metav1.Time{Time: expiredTime},
							},
						},
					},
				},
			},
			mockDeleter: &MockInstanceDeleter{
				err: errMockInstanceDelete,
			},
			wantNewKeysCount:    1, // Key should be kept if deletion fails
			wantErr:             true,
			wantDeleteCallCount: 1,
		},
		{
			name: "KeepKeysWithMissingName",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "expired-key",
								Name:        "", // Empty name should be kept
								CreatedDate: metav1.Time{Time: expiredTime},
								RetiredDate: metav1.Time{Time: expiredTime},
							},
						},
					},
				},
			},
			mockDeleter:         &MockInstanceDeleter{},
			wantNewKeysCount:    1,
			wantErr:             false,
			wantDeleteCallCount: 0,
		},
		{
			name: "NoExpiredKeys",
			cr: &v1alpha1.ServiceBinding{
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour},
							Frequency: &metav1.Duration{Duration: 30 * time.Minute},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "valid-key",
								Name:        "valid-name",
								CreatedDate: metav1.Time{Time: validTime},
								RetiredDate: metav1.Time{Time: time.Now()},
							},
						},
					},
				},
			},
			mockDeleter:         &MockInstanceDeleter{},
			wantNewKeysCount:    1,
			wantErr:             false,
			wantDeleteCallCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSBKeyRotator(tt.mockDeleter)
			mockDeleter := tt.mockDeleter.(*MockInstanceDeleter)

			gotKeys, err := r.DeleteExpiredKeys(context.Background(), tt.cr)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantNewKeysCount, len(gotKeys))
			assert.Equal(t, tt.wantDeleteCallCount, mockDeleter.deleteCallCount)
		})
	}
}

func TestSBKeyRotator_DeleteRetiredKeys(t *testing.T) {
	tests := []struct {
		name        string
		cr          *v1alpha1.ServiceBinding
		mockDeleter InstanceDeleter
		wantErr     bool
	}{
		{
			name: "DeleteAllRetiredKeysSuccessfully",
			cr: &v1alpha1.ServiceBinding{
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "key1",
								Name:        "name1",
								RetiredDate: metav1.Time{Time: time.Now().Add(-time.Hour)},
							},
							{
								ID:   "key2",
								Name: "name2",
							},
						},
					},
				},
			},
			mockDeleter: &MockInstanceDeleter{},
			wantErr:     false,
		},
		{
			name: "DeleteRetiredKeysWithError",
			cr: &v1alpha1.ServiceBinding{
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:          "key1",
								Name:        "name1",
								RetiredDate: metav1.Time{Time: time.Now().Add(-time.Hour)},
							},
						},
					},
				},
			},
			mockDeleter: &MockInstanceDeleter{
				err: errMockInstanceDelete,
			},
			wantErr: true,
		},
		{
			name: "NoRetiredKeys",
			cr: &v1alpha1.ServiceBinding{
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						RetiredKeys: []*v1alpha1.RetiredSBResource{},
					},
				},
			},
			mockDeleter: &MockInstanceDeleter{},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSBKeyRotator(tt.mockDeleter)
			mockDeleter := tt.mockDeleter.(*MockInstanceDeleter)

			err := r.DeleteRetiredKeys(context.Background(), tt.cr)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), errDeleteRetiredKey)
			} else {
				assert.NoError(t, err)
			}

			expectedCallCount := len(tt.cr.Status.AtProvider.RetiredKeys)
			assert.Equal(t, expectedCallCount, mockDeleter.deleteCallCount)
		})
	}
}

// Mock implementation for InstanceDeleter
var _ InstanceDeleter = &MockInstanceDeleter{}

type MockInstanceDeleter struct {
	err             error
	deleteCallCount int
}

func (m *MockInstanceDeleter) DeleteInstance(ctx context.Context, cr *v1alpha1.ServiceBinding, targetName string, targetExternalName string) error {
	m.deleteCallCount++
	return m.err
}
