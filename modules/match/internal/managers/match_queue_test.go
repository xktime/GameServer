package managers

import "testing"

func TestMatchQueueSettlingBlocksCancelUntilResolved(t *testing.T) {
	queue := newMatchQueue()
	queue.AddTeamRequest(&teamMatchRequest{TeamId: 7, PlayerIds: []int64{42}})

	if !queue.MarkSettling([]int64{7}, "match-1") {
		t.Fatal("MarkSettling() = false")
	}
	if requests := queue.GetTeamRequests(); len(requests) != 0 {
		t.Fatalf("settling requests remained matchable: %#v", requests)
	}
	if queue.RemoveTeamRequest(7) {
		t.Fatal("settling Team was cancelable")
	}
	if requeued := queue.RequeueMatch("match-1"); requeued != 1 {
		t.Fatalf("RequeueMatch() = %d", requeued)
	}
	if requests := queue.GetTeamRequests(); len(requests) != 1 || requests[0].TeamId != 7 {
		t.Fatalf("requeued requests = %#v", requests)
	}
	if !queue.MarkSettling([]int64{7}, "match-1") {
		t.Fatal("second MarkSettling() = false")
	}
	if removed := queue.RemoveSettledMatch("match-1"); removed != 1 {
		t.Fatalf("RemoveSettledMatch() = %d", removed)
	}
	if queue.IsTeamInQueue(7) || len(queue.GetTeamRequests()) != 0 {
		t.Fatalf("resolved queue = %#v", queue.TeamRequests)
	}
}
