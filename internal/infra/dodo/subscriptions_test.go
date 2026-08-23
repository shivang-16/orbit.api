package dodo

import (
	"errors"
	"testing"
)

func TestIsAlreadyCancelled(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("dodo request failed (404): missing"), true},
		{errors.New("subscription not found"), true},
		{errors.New("dodo request failed (422): subscription is already cancelled"), true},
		{errors.New("dodo request failed (422): subscription is inactive"), true},
		{errors.New("dodo request failed (422): pending plan change exists"), false},
		{errors.New("dodo request failed (422): already has a pending plan change"), false},
		{errors.New("dodo request failed (422): subscription already exists"), false},
		{errors.New("dodo request failed (422): invalid request"), false},
		{errors.New("dodo request failed (500): boom"), false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isAlreadyCancelled(tc.err); got != tc.want {
			t.Fatalf("isAlreadyCancelled(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
