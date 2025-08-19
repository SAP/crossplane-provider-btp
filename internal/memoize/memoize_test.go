package memoize

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"k8s.io/utils/ptr"
)

type testKey int

func key(k int) testKey {
	return testKey(k)
}

func (tv testKey) GetMemoKey() *string {
	return ptr.To(strconv.Itoa(int(tv)))
}

func TestMapWithIntValue(t *testing.T) {
	m := New[int](context.Background())

	_, found := m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("initially, the map shall not contain key(1313)")
	}

	m.Set(key(1313), ptr.To(35)) // store 35 with the key 1313

	value, found := m.Get(key(1313)) // get the value for key 1313
	if !found {
		t.Errorf("after stored, the map shall contain key(1313)")
	}
	if ptr.Deref(value, -1) != 35 {
		t.Errorf("after stored, the map shall contain the correct value for key(1313)")
	}

	_, found = m.Get(key(1314)) // get the value for key 1314
	if found {
		t.Errorf("the map shall not contain key(1314)")
	}

	m.Set(key(1313), nil)       // delete an existing key
	_, found = m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("after key(1313) is deleted, the map shall not contain key(1313)")
	}
	m.Set(key(1314), nil) // delete a non-existing key shall succeed
}

func TestMapKeyExpiration(t *testing.T) {
	m := New[int](context.Background()).WithKeyExpiration(0) // map key expire immediately

	_, found := m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("initially, the map shall not contain key(1313)")
	}

	m.Set(key(1313), ptr.To(35)) // store 35 with the key 1313

	_, found = m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("after stored, the map shall not contain key(1313) as it is expired")
	}

	_, found = m.Get(key(1314)) // get the value for key 1314
	if found {
		t.Errorf("after key(1313) stored, the map shall not contain key(1314)")
	}

	m.Set(key(1313), nil) // delete key
}

func TestMapStopped(t *testing.T) {
	m := New[int](context.Background())

	m.Stop()

	_, found := m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("a stopped map shall not contain key(1313)")
	}

	m.Set(key(1313), ptr.To(35)) // store 35 with the key 1313

	_, found = m.Get(key(1313)) // get the value for key 1313
	if found {
		t.Errorf("a stopped map shall not contain key(1313) even it was stored before")
	}

	m.Set(key(1313), nil) // delete key
}

func TestMapKeyLimit(t *testing.T) {
	m := New[int](context.Background()).WithKeyLimit(3)

	m.Set(key(1), ptr.To(1))
	m.Set(key(2), ptr.To(2))
	m.Set(key(3), ptr.To(3))
	m.Set(key(4), ptr.To(4))
	m.Set(key(5), ptr.To(5))

	_, found := m.Get(key(1)) // check key 1
	if !found {
		t.Errorf("key 1 shall be present")
	}

	_, found = m.Get(key(2)) // check key 2
	if !found {
		t.Errorf("key 2 shall be present")
	}

	_, found = m.Get(key(3)) // check key 3
	if !found {
		t.Errorf("key 3 shall be present")
	}

	_, found = m.Get(key(4)) // check key 4
	if found {
		t.Errorf("key 4 shall not be present")
	}

	_, found = m.Get(key(5)) // check key 5
	if found {
		t.Errorf("key 5 shall not be present")
	}
}

func ExampleMap() {
	m := New[int](context.Background())

	m.Set(testKey(15), ptr.To(5))
	value, found := m.Get(testKey(15))
	fmt.Printf("%d, %t", *value, found)
	// Output: 5, true
}
