package controllers

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

func TestParseScheduleNext(t *testing.T) {
	cases := []struct {
		expr string
		from time.Time
		want time.Time
	}{
		{"* * * * *", time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC), time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC)},
		{"*/5 * * * *", time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC), time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)},
		{"0 0 * * *", time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{"30 2 * * mon", time.Date(2026, 1, 5, 3, 0, 0, 0, time.UTC), time.Date(2026, 1, 12, 2, 30, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		s, err := ParseSchedule(c.expr)
		if err != nil {
			t.Fatalf("ParseSchedule(%q): %v", c.expr, err)
		}
		if got := s.Next(c.from); !got.Equal(c.want) {
			t.Errorf("Next(%q, %v) = %v, want %v", c.expr, c.from, got, c.want)
		}
	}

	if _, err := ParseSchedule("bogus"); err == nil {
		t.Error("invalid schedule must fail")
	}
	if s, err := ParseSchedule("@every 1h"); err != nil {
		t.Fatalf("@every: %v", err)
	} else if got := s.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)) {
		t.Errorf("@every Next = %v", got)
	}
}

// A CronJob whose schedule has passed gets a Job created from its template,
// with a default backoffLimit, and the run is recorded in its status.
func TestCronJobControllerCreatesJob(t *testing.T) {
	s := store.NewMemoryStore()
	c := &CronJobController{store: s, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	limit := int32(2)
	cj := k8s.CronJob{
		TypeMeta:   k8s.TypeMeta{Kind: "CronJob", APIVersion: "batch/v1"},
		ObjectMeta: k8s.ObjectMeta{Name: "nightly", Namespace: "default", CreationTimestamp: time.Now().Add(-2 * time.Hour)},
		Spec: k8s.CronJobSpec{
			Schedule: "* * * * *", // due on every tick
			JobTemplate: k8s.JobTemplateSpec{
				Spec: k8s.JobSpec{
					BackoffLimit: &limit,
					Template:     k8s.PodTemplateSpec{},
				},
			},
		},
	}
	data, _ := json.Marshal(cj)
	if err := s.Put(store.KeyFor("batch", "cronjobs", "default", "nightly"), &store.StoredObject{Value: data}); err != nil {
		t.Fatal(err)
	}

	c.tick(time.Now())

	objects, _ := s.List(store.KeyFor("batch", "jobs", "default", ""))
	if len(objects) != 1 {
		t.Fatalf("created %d jobs, want 1", len(objects))
	}
	var job k8s.Job
	if err := json.Unmarshal(objects[0].Value, &job); err != nil {
		t.Fatal(err)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 2 {
		t.Errorf("backoffLimit = %v, want 2", job.Spec.BackoffLimit)
	}

	// The run is recorded: a second tick with the same "now" must not create
	// another job, and status.active must reference the created job.
	obj, err := s.Get(store.KeyFor("batch", "cronjobs", "default", "nightly"))
	if err != nil {
		t.Fatal(err)
	}
	var updated k8s.CronJob
	_ = json.Unmarshal(obj.Value, &updated)
	if updated.Status.LastScheduleTime == nil {
		t.Fatal("lastScheduleTime not recorded")
	}
	if len(updated.Status.Active) != 1 || updated.Status.Active[0].Name != job.Name {
		t.Errorf("active = %+v, want the created job", updated.Status.Active)
	}

	c.tick(*updated.Status.LastScheduleTime)
	objects, _ = s.List(store.KeyFor("batch", "jobs", "default", ""))
	if len(objects) != 1 {
		t.Fatalf("tick re-created a job for the same schedule time: %d jobs", len(objects))
	}
}

// A suspended CronJob never creates Jobs.
func TestCronJobControllerSuspended(t *testing.T) {
	s := store.NewMemoryStore()
	c := &CronJobController{store: s, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	suspend := true
	cj := k8s.CronJob{
		ObjectMeta: k8s.ObjectMeta{Name: "paused", Namespace: "default", CreationTimestamp: time.Now().Add(-2 * time.Hour)},
		Spec:       k8s.CronJobSpec{Schedule: "* * * * *", Suspend: &suspend},
	}
	data, _ := json.Marshal(cj)
	_ = s.Put(store.KeyFor("batch", "cronjobs", "default", "paused"), &store.StoredObject{Value: data})

	c.tick(time.Now())
	if objects, _ := s.List(store.KeyFor("batch", "jobs", "default", "")); len(objects) != 0 {
		t.Fatalf("suspended cronjob created %d jobs", len(objects))
	}
}
