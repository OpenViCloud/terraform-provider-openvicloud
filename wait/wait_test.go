package wait

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeClock struct {
	now   time.Time
	slept []time.Duration
}

func (f *fakeClock) clock() Clock {
	return Clock{
		Now:   func() time.Time { return f.now },
		Sleep: func(d time.Duration) { f.slept = append(f.slept, d); f.now = f.now.Add(d) },
	}
}

func TestUntil_DoneOnTheThirdCheck_SleepsTwice(t *testing.T) {
	fake := &fakeClock{now: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	calls := 0

	err := Until(context.Background(), fake.clock(), time.Minute, 5*time.Second, func(ctx context.Context) (bool, error) {
		calls++
		return calls == 3, nil
	})

	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 3 || len(fake.slept) != 2 || fake.slept[0] != 5*time.Second {
		t.Fatalf("calls = %d, slept = %v", calls, fake.slept)
	}
}

func TestUntil_NeverDone_StopsAtTheTimeout(t *testing.T) {
	fake := &fakeClock{now: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	calls := 0

	err := Until(context.Background(), fake.clock(), 20*time.Second, 5*time.Second, func(ctx context.Context) (bool, error) {
		calls++
		return false, nil
	})

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	// Checks at 0, 5, 10, 15 and 20 seconds; the one at 20 is the last.
	if calls != 5 {
		t.Fatalf("calls = %d, want 5", calls)
	}
}

func TestUntil_CheckFails_ReturnsItsErrorAtOnce(t *testing.T) {
	fake := &fakeClock{now: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	refused := errors.New("refused")

	err := Until(context.Background(), fake.clock(), time.Minute, 5*time.Second, func(ctx context.Context) (bool, error) {
		return false, refused
	})

	if !errors.Is(err, refused) || len(fake.slept) != 0 {
		t.Fatalf("err = %v, slept = %v", err, fake.slept)
	}
}
