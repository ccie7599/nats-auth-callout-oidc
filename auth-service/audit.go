package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// AuditEvent represents an authentication/authorization decision.
type AuditEvent struct {
	Timestamp   time.Time     `json:"timestamp"`
	UserNKey    string        `json:"user_nkey"`
	ClientIP    string        `json:"client_ip"`
	TokenIssuer string        `json:"token_issuer,omitempty"`
	TokenSub    string        `json:"token_sub,omitempty"`
	Scopes      []string      `json:"scopes,omitempty"`
	Decision    string        `json:"decision"`
	Reason      string        `json:"reason,omitempty"`
	Permissions *GrantedPerms `json:"permissions,omitempty"`
}

// GrantedPerms represents the NATS permissions granted to a user.
type GrantedPerms struct {
	PubAllow []string `json:"pub_allow,omitempty"`
	SubAllow []string `json:"sub_allow,omitempty"`
}

// AuditPublisher publishes auth decision events to NATS.
type AuditPublisher struct {
	nc *nats.Conn
}

// NewAuditPublisher creates a new audit publisher.
func NewAuditPublisher(nc *nats.Conn) *AuditPublisher {
	return &AuditPublisher{nc: nc}
}

// PublishSuccess publishes a successful auth event.
func (a *AuditPublisher) PublishSuccess(event AuditEvent) {
	event.Decision = "success"
	event.Timestamp = time.Now().UTC()
	a.publish("auth.audit.success", event)
}

// PublishFailure publishes a failed auth event.
func (a *AuditPublisher) PublishFailure(event AuditEvent) {
	event.Decision = "failure"
	event.Timestamp = time.Now().UTC()
	a.publish("auth.audit.failure", event)
}

func (a *AuditPublisher) publish(subject string, event AuditEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal audit event: %v", err)
		return
	}
	if err := a.nc.Publish(subject, data); err != nil {
		log.Printf("Failed to publish audit event to %s: %v", subject, err)
	}
}
