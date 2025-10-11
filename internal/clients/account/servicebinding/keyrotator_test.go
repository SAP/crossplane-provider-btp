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
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour * 24},
							Frequency: &metav1.Duration{Duration: time.Hour * 6},
						},
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
				Spec: v1alpha1.ServiceBindingSpec{
					ForProvider: v1alpha1.ServiceBindingParameters{
						Rotation: &v1alpha1.RotationParameters{
							TTL:       &metav1.Duration{Duration: time.Hour * 24},
							Frequency: &metav1.Duration{Duration: time.Hour * 6},
						},
					},
				},
				Status: v1alpha1.ServiceBindingStatus{
					AtProvider: v1alpha1.ServiceBindingObservation{
						ID:   "current-id",
						Name: "current-name",
						RetiredKeys: []*v1alpha1.RetiredSBResource{
							{
								ID:           "current-id",
								Name:         "current-name",
								CreatedDate:  metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
								RetiredDate:  metav1.Time{Time: time.Now().Add(-time.Hour)},
								DeletionDate: &metav1.Time{Time: time.Now().Add(time.Hour)},
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
							TTL:       &metav1.Duration{Duration: time.Hour * 24},
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
							TTL:       &metav1.Duration{Duration: time.Hour * 24},
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
							TTL:       &metav1.Duration{Duration: time.Hour * 24},
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
				// CreatedDate should be preserved when it exists
				if tt.name == "RotationDue_NewRetirement" {
					assert.NotNil(t, tt.cr.Status.AtProvider.CreatedDate)
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
								ID:           "expired-key",
								Name:         "expired-name",
								CreatedDate:  metav1.Time{Time: expiredTime},
								RetiredDate:  metav1.Time{Time: expiredTime},
								DeletionDate: &metav1.Time{Time: expiredTime.Add(30 * time.Minute)},
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
								ID:           "valid-key",
								Name:         "valid-name",
								CreatedDate:  metav1.Time{Time: validTime},
								RetiredDate:  metav1.Time{Time: time.Now()},
								DeletionDate: &metav1.Time{Time: time.Now().Add(30 * time.Minute)},
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
								ID:           "some-key",
								Name:         "some-name",
								CreatedDate:  metav1.Time{Time: expiredTime},
								RetiredDate:  metav1.Time{Time: expiredTime},
								DeletionDate: nil, // Will be updated by the new logic
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
								ID:           "invalid-key",
								Name:         "invalid-name",
								CreatedDate:  metav1.Time{}, // invalid date case
								RetiredDate:  metav1.Time{}, // invalid date case
								DeletionDate: nil,          // Will be updated by the new logic
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
								ID:           "expired-key",
								Name:         "expired-name",
								CreatedDate:  metav1.Time{Time: expiredTime},
								RetiredDate:  metav1.Time{Time: expiredTime},
								DeletionDate: &metav1.Time{Time: expiredTime.Add(30 * time.Minute)},
							},
							{
								ID:           "valid-key",
								Name:         "valid-name",
								CreatedDate:  metav1.Time{Time: validTime},
								RetiredDate:  metav1.Time{Time: time.Now()},
								DeletionDate: &metav1.Time{Time: time.Now().Add(30 * time.Minute)},
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
								ID:           "expired-key",
								Name:         "expired-name",
								CreatedDate:  metav1.Time{Time: expiredTime},
								RetiredDate:  metav1.Time{Time: expiredTime},
								DeletionDate: &metav1.Time{Time: expiredTime.Add(30 * time.Minute)},
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
								ID:           "valid-key",
								Name:         "valid-name",
								CreatedDate:  metav1.Time{Time: validTime},
								RetiredDate:  metav1.Time{Time: time.Now()},
								DeletionDate: &metav1.Time{Time: time.Now().Add(30 * time.Minute)},
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
								ID:           "key1",
								Name:         "name1",
								CreatedDate:  metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
								RetiredDate:  metav1.Time{Time: time.Now().Add(-time.Hour)},
								DeletionDate: nil,
							},
							{
								ID:           "key2",
								Name:         "name2",
								CreatedDate:  metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
								RetiredDate:  metav1.Time{},
								DeletionDate: nil,
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
								ID:           "key1",
								Name:         "name1",
								CreatedDate:  metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
								RetiredDate:  metav1.Time{Time: time.Now().Add(-time.Hour)},
								DeletionDate: nil,
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

func TestDeletionDate(t *testing.T) {
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		baseTime     time.Time
		rotation     *v1alpha1.RotationParameters
		expectedTime time.Time
	}{
		{
			name:     "Normal case with TTL 24h and Frequency 6h",
			baseTime: baseTime,
			rotation: &v1alpha1.RotationParameters{
				TTL:       &metav1.Duration{Duration: 24 * time.Hour},
				Frequency: &metav1.Duration{Duration: 6 * time.Hour},
			},
			expectedTime: baseTime.Add(18 * time.Hour), // TTL - Frequency = 24h - 6h = 18h
		},
		{
			name:     "Edge case with TTL equal to Frequency",
			baseTime: baseTime,
			rotation: &v1alpha1.RotationParameters{
				TTL:       &metav1.Duration{Duration: 6 * time.Hour},
				Frequency: &metav1.Duration{Duration: 6 * time.Hour},
			},
			expectedTime: baseTime, // TTL - Frequency = 6h - 6h = 0h
		},
		{
			name:     "Large TTL with small Frequency",
			baseTime: baseTime,
			rotation: &v1alpha1.RotationParameters{
				TTL:       &metav1.Duration{Duration: 7 * 24 * time.Hour}, // 7 days
				Frequency: &metav1.Duration{Duration: 1 * time.Hour},
			},
			expectedTime: baseTime.Add(7*24*time.Hour - time.Hour), // 7 days - 1 hour
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deletionDate(tt.baseTime, tt.rotation)
			assert.Equal(t, tt.expectedTime, result.Time)
		})
	}
}
