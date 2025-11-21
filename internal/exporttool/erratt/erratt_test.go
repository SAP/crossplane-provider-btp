package erratt_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sap/crossplane-provider-btp/internal/exporttool/erratt"
)

var _ = Describe("Erratt", func() {
	var (
		errStr string
		ea     erratt.Error
	)

	BeforeEach(func() {
		errStr = "string error"
	})
	Describe("New method", func() {
		Context("with a single string attribute", func() {
			BeforeEach(func() {
				ea = erratt.New(errStr)
			})
			It("generates a valid ErrAtt with zero attributes", func() {
				Expect(len(ea.Attrs())).To(Equal(0))
			})
			It("generates a valid ErrAtt with the expected error string", func() {
				Expect(ea.Error()).To(Equal(errStr))
			})
		})
		Context("with a text and a valid string attribute", func() {
			It("contains the attribute", func() {
				ea = erratt.New(errStr, "key", "value")
				Expect(len(ea.Attrs())).To(Equal(2))
				Expect(ea.Attrs()[0]).To(Equal("key"))
				Expect(ea.Attrs()[1]).To(Equal("value"))
			})
		})
		Context("with a text and a valid int attribute", func() {
			It("contains the attribute", func() {
				ea = erratt.New(errStr, "key", 11)
				Expect(len(ea.Attrs())).To(Equal(2))
				Expect(ea.Attrs()[0]).To(Equal("key"))
				Expect(ea.Attrs()[1]).To(Equal(11))
			})
		})
	})
	Describe("Errorf function", func() {
		Context("with a single string", func() {
			var ea erratt.Error
			Context("and no attributes", func() {
				BeforeEach(func() {
					ea = erratt.Errorf("test error")
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("test error"))
				})
				It("contains no wrapped errors", func() {
					Expect(ea.Unwrap()).To(BeNil())
				})
				It("contains zero attributes", func() {
					Expect(ea.Attrs()).To(BeNil())
				})
			})
			Context("and a single string attribute", func() {
				BeforeEach(func() {
					ea = erratt.Errorf("test error").With("key", "value")
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("test error"))
				})
				It("contains no wrapped errors", func() {
					Expect(ea.Unwrap()).To(BeNil())
				})
				It("contains the attribute", func() {
					Expect(len(ea.Attrs())).To(Equal(2))
					Expect(ea.Attrs()[0]).To(Equal("key"))
					Expect(ea.Attrs()[1]).To(Equal("value"))
				})
			})
			Context("and a single int attribute", func() {
				BeforeEach(func() {
					ea = erratt.Errorf("test error").With("key", 11)
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("test error"))
				})
				It("contains no wrapped errors", func() {
					Expect(ea.Unwrap()).To(BeNil())
				})
				It("contains the attribute", func() {
					Expect(len(ea.Attrs())).To(Equal(2))
					Expect(ea.Attrs()[0]).To(Equal("key"))
					Expect(ea.Attrs()[1]).To(Equal(11))
				})
			})
		})
		Context("with a wrapped simple error", func() {
			var simpleWrappedError error
			BeforeEach(func() {
				simpleWrappedError = errors.New("wrapped error")
				ea = erratt.Errorf("this error wraps: %w", simpleWrappedError)
			})
			Context("and no attributes", func() {
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("this error wraps: wrapped error"))
				})
				It("contains the wrapped error", func() {
					Expect(ea.Unwrap()).To(Equal(simpleWrappedError))
				})
				It("contains zero attributes", func() {
					Expect(ea.Attrs()).To(BeNil())
				})
			})
			Context("and a single string attribute", func() {
				BeforeEach(func() {
					ea = erratt.
						Errorf("this error wraps: %w", simpleWrappedError).
						With("key", "value")
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("this error wraps: wrapped error"))
				})
				It("contains no wrapped errors", func() {
					Expect(ea.Unwrap()).To(Equal(simpleWrappedError))
				})
				It("contains the attribute", func() {
					Expect(len(ea.Attrs())).To(Equal(2))
					Expect(ea.Attrs()[0]).To(Equal("key"))
					Expect(ea.Attrs()[1]).To(Equal("value"))
				})
			})
			Context("and a single int attribute", func() {
				BeforeEach(func() {
					ea = erratt.
						Errorf("this error wraps: %w", simpleWrappedError).
						With("key", 11)
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("this error wraps: wrapped error"))
				})
				It("contains no wrapped errors", func() {
					Expect(ea.Unwrap()).To(Equal(simpleWrappedError))
				})
				It("contains the attribute", func() {
					Expect(len(ea.Attrs())).To(Equal(2))
					Expect(ea.Attrs()[0]).To(Equal("key"))
					Expect(ea.Attrs()[1]).To(Equal(11))
				})
			})
		})
		Context("with a wrapped erratt error", func() {
			var wrappedErratt erratt.Error
			BeforeEach(func() {
				wrappedErratt = erratt.New("wrapped", "key", "value")
			})
			Context("and no attributes", func() {
				BeforeEach(func() {
					ea = erratt.Errorf("outer: %w", wrappedErratt)
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("outer: wrapped"))
				})
				It("contains the wrapped error", func() {
					Expect(ea.Unwrap()).To(Equal(wrappedErratt))
				})
				It("contains the wrapped attributes", func() {
					Expect(len(ea.Attrs())).To(Equal(2))
					Expect(ea.Attrs()[0]).To(Equal("key"))
					Expect(ea.Attrs()[1]).To(Equal("value"))
				})
			})
			Context("and a single string attribute", func() {
				BeforeEach(func() {
					ea = erratt.
						Errorf("outer: %w", wrappedErratt).
						With("outer-key", "outer-value")
				})
				It("has the proper message", func() {
					Expect(ea.Error()).To(Equal("outer: wrapped"))
				})
				It("contains the wrapped error", func() {
					Expect(ea.Unwrap()).To(Equal(wrappedErratt))
				})
				It("contains both attributes", func() {
					Expect(len(ea.Attrs())).To(Equal(4))
					Expect(ea.Attrs()[0]).To(Equal("outer-key"))
					Expect(ea.Attrs()[1]).To(Equal("outer-value"))
					Expect(ea.Attrs()[2]).To(Equal("key"))
					Expect(ea.Attrs()[3]).To(Equal("value"))
				})
			})
		})
	})
})
