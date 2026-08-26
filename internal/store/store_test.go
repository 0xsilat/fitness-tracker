package store

import "testing"

func TestDecodePrescriptionConvertsLegacyPyramids(t *testing.T){
	raw:=[]byte(`{"format":"mixed","movements":[{"exercise_id":1,"exercise_name":"Squat","mode":"weighted","format":"ascending_pyramid","sets":[{"reps":5,"weight_kg":80},{"reps":8,"weight_kg":70}]}]}`)
	p,err:=decodePrescription(raw);if err!=nil{t.Fatal(err)}
	if got:=p.Movements[0].Format;got!="sets_reps"{t.Fatalf("format=%q",got)}
	if p.Movements[0].Sets[0].Reps!=5||p.Movements[0].Sets[1].WeightKG!=70{t.Fatalf("numeric plan changed: %#v",p.Movements[0].Sets)}
}

func TestDecodePrescriptionDefaultsLegacyEMOMInterval(t *testing.T){
	raw:=[]byte(`{"movements":[{"exercise_id":1,"exercise_name":"Burpee","mode":"bodyweight","format":"emom","duration_minutes":10,"sets":[{"reps":0,"minute":1}]}]}`)
	p,err:=decodePrescription(raw);if err!=nil{t.Fatal(err)}
	if got:=p.Movements[0].IntervalMinutes;got!=1{t.Fatalf("interval=%d",got)}
}
