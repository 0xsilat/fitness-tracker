package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOptionalInt(t *testing.T) {
	got,err:=optionalInt("12");if err!=nil||got!=12{t.Fatalf("got=%d err=%v",got,err)}
	got,err=optionalInt("");if err!=nil||got!=0{t.Fatalf("blank got=%d err=%v",got,err)}
	if _,err:=optionalInt("nope");err==nil{t.Fatal("expected invalid number error")}
	if _,err:=optionalInt("-1");err==nil{t.Fatal("expected negative error")}
}

func TestOriginMiddleware(t *testing.T) {
	next:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(http.StatusNoContent)})
	handler:=originMiddleware(next)
	allowed:=httptest.NewRequest(http.MethodPost,"http://localhost/exercises",nil)
	allowed.Host="localhost";allowed.Header.Set("Origin","http://localhost")
	w:=httptest.NewRecorder();handler.ServeHTTP(w,allowed)
	if w.Code!=http.StatusNoContent{t.Fatalf("same origin status=%d",w.Code)}

	blocked:=httptest.NewRequest(http.MethodPost,"http://localhost/exercises",nil)
	blocked.Host="localhost";blocked.Header.Set("Origin","https://attacker.example")
	w=httptest.NewRecorder();handler.ServeHTTP(w,blocked)
	if w.Code!=http.StatusForbidden{t.Fatalf("cross origin status=%d",w.Code)}
}
