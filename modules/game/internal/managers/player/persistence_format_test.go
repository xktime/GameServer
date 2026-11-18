package player

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestPlayerPersistenceKeepsCollectionNameAndBSONFields(t *testing.T) {
	legacy := bson.M{"_id": int64(42), "player_info": nil, "team_id": int64(7), "tower_level": int32(3)}
	encoded, err := bson.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var data Player
	if err := bson.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if name := reflect.TypeOf(data).Name(); name != "Player" {
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
