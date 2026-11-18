package team

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestTeamPersistenceKeepsCollectionNameAndBSONFields(t *testing.T) {
	legacy := bson.M{"_id": int64(7), "leader_id": int64(42), "team_members": bson.A{int64(42)}, "room_id": "room-1"}
	encoded, err := bson.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var data Team
	if err := bson.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if name := reflect.TypeOf(data).Name(); name != "Team" {
		t.Fatalf("Mongo collection changed to %s", name)
	}
	encoded, err = bson.Marshal(&data)
	if err != nil {
		t.Fatal(err)
	}
	var stored bson.M
	if err := bson.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, legacy) {
		t.Fatalf("persisted fields changed: %#v", stored)
	}
}
