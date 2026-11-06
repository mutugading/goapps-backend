package grpc

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	"github.com/mutugading/goapps-backend/services/payroll/internal/application/run"
)

var nominalField = regexp.MustCompile(`amount|total|take_home|^base$|^rate$`)

// walkNominals visits every string field of m (recursively) whose name looks like a nominal.
func walkNominals(m protoreflect.Message, path string, visit func(path string, v string, populated bool)) {
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		name := string(fd.Name())
		switch {
		case fd.Kind() == protoreflect.StringKind && !fd.IsList() && !fd.IsMap() && nominalField.MatchString(name):
			visit(path+"."+name, m.Get(fd).String(), true)
		case fd.Kind() == protoreflect.MessageKind && fd.IsList():
			l := m.Get(fd).List()
			for j := 0; j < l.Len(); j++ {
				walkNominals(l.Get(j).Message(), path+"."+name, visit)
			}
		case fd.Kind() == protoreflect.MessageKind && !fd.IsMap() && m.Has(fd):
			walkNominals(m.Get(fd).Message(), path+"."+name, visit)
		}
	}
}

// TestMaskRunResponses fails when a nominal string field of any run RPC response is left unmasked, including
// fields added to the proto later whose builder forgot mask().
func TestMaskRunResponses(t *testing.T) {
	info := run.Info{ID: 1, YYMM: 202611, Division: "SOLO", TotT: "1.00", TotP: "2.00", Status: "COMMITTED",
		Warnings: []run.Issue{{Code: "LOAN_NET_NEGATIVE", NIK: "N1", Message: "net -1234567.89 setelah cicilan"}}, StartedAt: time.Now(), FinishedAt: time.Now()}
	line := run.LineInfo{NIK: "N1", CompType: "T", CompID: 1, Amount: "3.00", Base: "4.00", Qty: "1", Rate: "5.00"}
	emp := run.EmployeeTotal{NIK: "N1", TotT: "1.00", TotP: "2.00", TakeHome: "9.00"}
	resps := map[string]proto.Message{
		"CreateRun(dry)": &payrollv1.CreateRunResponse{Data: runPB(info, false), Employees: employeesPB([]run.EmployeeTotal{emp}, false)},
		"GetRun":         &payrollv1.GetRunResponse{Data: runPB(info, false)},
		"ListRuns":       &payrollv1.ListRunsResponse{Data: []*payrollv1.Run{runPB(info, false)}},
		"ListRunLines":   &payrollv1.ListRunLinesResponse{Data: []*payrollv1.RunLine{linePB(line, false)}},
	}
	for name, msg := range resps {
		seen := 0
		walkNominals(msg.ProtoReflect(), name, func(path, v string, _ bool) {
			seen++
			if v != masked {
				t.Errorf("%s = %q, want %q (nominal not masked without %s)", path, v, masked, permSalaryAmount)
			}
		})
		if seen == 0 {
			t.Errorf("%s: no nominal field found, reflection walk is broken", name)
		}
	}
	if w := runPB(info, false).GetWarnings(); len(w) != 1 || strings.ContainsAny(w[0], "0123456789") && !strings.Contains(w[0], "N1") ||
		strings.Contains(w[0], "1234567") || !strings.Contains(w[0], "LOAN_NET_NEGATIVE") {
		t.Fatalf("masked warnings leak or lost code: %v", w)
	}
	// With permission the same builders must pass values through.
	if got := linePB(line, true); got.GetAmount() != "3.00" || got.GetBase() != "4.00" || got.GetRate() != "5.00" || got.GetQty() != "1" {
		t.Fatalf("visible line = %+v", got)
	}
	r := runPB(info, true)
	if r.GetTotalTunjangan() != "1.00" || r.GetTotalPotongan() != "2.00" || r.GetDivision() != "SOLO" || r.GetStatus() != "COMMITTED" ||
		len(r.GetWarnings()) != 1 || !strings.Contains(r.GetWarnings()[0], "-1234567.89") {
		t.Fatalf("visible run = %+v", r)
	}
	if e := employeesPB([]run.EmployeeTotal{emp}, true)[0]; e.GetTotalTunjangan() != "1.00" || e.GetTotalPotongan() != "2.00" || e.GetTakeHome() != "9.00" || e.GetNik() != "N1" {
		t.Fatalf("visible employee = %+v", e)
	}
	if got := linePB(line, true); got.GetNik() != "N1" || got.GetCompType() != "T" || got.GetCompId() != 1 {
		t.Fatalf("visible line non-nominal = %+v", got)
	}
	m := runPB(info, false)
	if m.GetDivision() != "SOLO" || m.GetStatus() != "COMMITTED" || m.GetRunId() != 1 {
		t.Fatalf("masked run lost non-nominal fields: %+v", m)
	}
}
