package bus

import (
	"fmt"

	"github.com/paseka/paseka/internal/colony"
)

// ConnectColony connects to NATS using resolved colony context.
// Returns nil client when NATS URL is not configured.
func ConnectColony(ctx colony.Context, full bool) (*Client, error) {
	manifest, err := colony.LoadColony(ctx.ColonyRoot)
	if err != nil {
		return nil, err
	}
	cfg := ConfigFromContext(ctx, manifest)
	if !cfg.Enabled() {
		return nil, nil
	}
	if full {
		return ConnectFull(cfg)
	}
	return Connect(cfg)
}

// DoctorReport summarizes NATS health for one colony.
type DoctorReport struct {
	URL           string
	SubjectPrefix string
	Connected     bool
	JetStreamOK   bool
	StreamOK      bool
	KVOK          bool
	ObjectStoreOK bool
	Errors        []string
	Warnings      []string
	Advisories    []string
}

// Diagnose checks NATS connectivity and JetStream resources.
func Diagnose(ctx colony.Context) (DoctorReport, error) {
	manifest, err := colony.LoadColony(ctx.ColonyRoot)
	if err != nil {
		return DoctorReport{}, err
	}
	cfg := ConfigFromContext(ctx, manifest)
	report := DoctorReport{
		URL:           cfg.URL,
		SubjectPrefix: cfg.SubjectPrefix,
	}
	if !cfg.Enabled() {
		report.Errors = append(report.Errors, "nats url not configured in home config")
		return report, nil
	}
	client, err := ConnectFull(cfg)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
		return report, nil
	}
	defer client.Close()

	report.Connected = client.nc.IsConnected()
	if err := client.Health(); err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		report.JetStreamOK = true
		report.StreamOK = true
	}
	if _, err := client.js.KeyValue(kvBucketName(cfg.Slug)); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("kv bucket: %v", err))
	} else {
		report.KVOK = true
	}
	if _, err := client.js.ObjectStore(objectStoreName(cfg.Slug)); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("object store: %v", err))
	} else {
		report.ObjectStoreOK = true
	}

	bees, err := colony.LoadAllBeesForDiagnosis(ctx.ColonyRoot)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		proposal := colony.DiagnoseCodeProposal(bees)
		report.Errors = append(report.Errors, proposal.Errors...)
		report.Warnings = append(report.Warnings, proposal.Warnings...)
		report.Advisories = append(report.Advisories, proposal.Advisories...)
	}
	return report, nil
}
