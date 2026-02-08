package governance

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// saveRaft persists a raft to the database
func (g *Governance) saveRaft(ctx context.Context, raft *RaftInfo) error {
	db := g.getDB()
	if db == nil {
		return fmt.Errorf("database not available")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or update raft
	_, err = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO governance_rafts (raft_id, created_at, updated_at)
		VALUES (?, ?, ?)
	`, raft.RaftID, raft.CreatedAt.Unix(), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to save raft: %w", err)
	}

	// Save all members of this raft
	raft.mu.RLock()
	for _, member := range raft.Members {
		var expiresAt *int64
		if member.ExpiresAt != nil {
			exp := member.ExpiresAt.Unix()
			expiresAt = &exp
		}

		_, err = tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO governance_members 
			(raft_id, member_id, state, joined_at, last_seen_at, public_key, signature, inducted_by, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, raft.RaftID, member.ID, string(member.State), member.JoinedAt.Unix(),
			member.LastSeenAt.Unix(), member.PublicKey, member.Signature, member.InductedBy, expiresAt)
		if err != nil {
			raft.mu.RUnlock()
			return fmt.Errorf("failed to save member: %w", err)
		}
	}

	// Save all rules of this raft
	for _, rule := range raft.Rules {
		if err := g.saveRuleInTx(ctx, tx, rule); err != nil {
			raft.mu.RUnlock()
			return err
		}
	}
	raft.mu.RUnlock()

	return tx.Commit()
}

// saveRule persists a rule to the database
func (g *Governance) saveRule(ctx context.Context, rule *Rule) error {
	db := g.getDB()
	if db == nil {
		return fmt.Errorf("database not available")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := g.saveRuleInTx(ctx, tx, rule); err != nil {
		return err
	}

	return tx.Commit()
}

// saveRuleInTx saves a rule within an existing transaction
func (g *Governance) saveRuleInTx(ctx context.Context, tx *sql.Tx, rule *Rule) error {
	var adoptedAt *int64
	if rule.AdoptedAt != nil {
		adopted := rule.AdoptedAt.Unix()
		adoptedAt = &adopted
	}

	var baseRuleID *string
	if rule.BaseRuleID != "" {
		baseRuleID = &rule.BaseRuleID
	}

	_, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO governance_rules 
		(rule_id, raft_id, scope, version, timestamp, body, base_rule_id, signature, proposed_by, adopted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rule.RuleID, rule.RaftID, rule.Scope, rule.Version, rule.Timestamp.Unix(),
		rule.Body, baseRuleID, rule.Signature, rule.ProposedBy, adoptedAt)

	if err != nil {
		return fmt.Errorf("failed to save rule: %w", err)
	}

	return nil
}

// loadGovernanceState loads all persisted governance data
func (g *Governance) loadGovernanceState(ctx context.Context) error {
	db := g.getDB()
	if db == nil {
		return fmt.Errorf("database not available")
	}

	// Load all rafts
	rows, err := db.QueryContext(ctx, `SELECT raft_id, created_at FROM governance_rafts`)
	if err != nil {
		return fmt.Errorf("failed to query rafts: %w", err)
	}
	defer rows.Close()

	var raftIDs []string
	raftCreatedAt := make(map[string]time.Time)

	for rows.Next() {
		var raftID string
		var createdAt int64
		if err := rows.Scan(&raftID, &createdAt); err != nil {
			return fmt.Errorf("failed to scan raft: %w", err)
		}
		raftIDs = append(raftIDs, raftID)
		raftCreatedAt[raftID] = time.Unix(createdAt, 0)
	}
	rows.Close()

	// Load each raft with its members and rules
	for _, raftID := range raftIDs {
		// Skip if this raft already exists (e.g., the self raft created during bootstrap)
		g.rafts.mu.RLock()
		_, exists := g.rafts.rafts[raftID]
		g.rafts.mu.RUnlock()

		if exists && raftID == g.config.ID {
			// Skip loading self raft as it was just bootstrapped
			continue
		}

		raft := &RaftInfo{
			RaftID:    raftID,
			CreatedAt: raftCreatedAt[raftID],
			Members:   make(map[string]*Member),
			Rules:     make(map[string]*Rule),
		}

		// Load members
		memberRows, err := db.QueryContext(ctx, `
			SELECT member_id, state, joined_at, last_seen_at, public_key, signature, inducted_by, expires_at
			FROM governance_members WHERE raft_id = ?
		`, raftID)
		if err != nil {
			return fmt.Errorf("failed to query members for raft %s: %w", raftID, err)
		}

		for memberRows.Next() {
			var memberID, state, inductedBy string
			var joinedAt, lastSeenAt int64
			var publicKey, signature []byte
			var expiresAt *int64

			err := memberRows.Scan(&memberID, &state, &joinedAt, &lastSeenAt, &publicKey, &signature, &inductedBy, &expiresAt)
			if err != nil {
				memberRows.Close()
				return fmt.Errorf("failed to scan member: %w", err)
			}

			member := &Member{
				ID:         memberID,
				State:      MembershipState(state),
				JoinedAt:   time.Unix(joinedAt, 0),
				LastSeenAt: time.Unix(lastSeenAt, 0),
				PublicKey:  publicKey,
				Signature:  signature,
				InductedBy: inductedBy,
			}

			if expiresAt != nil {
				expires := time.Unix(*expiresAt, 0)
				member.ExpiresAt = &expires
			}

			raft.Members[memberID] = member
		}
		memberRows.Close()

		// Load rules
		ruleRows, err := db.QueryContext(ctx, `
			SELECT rule_id, raft_id, scope, version, timestamp, body, base_rule_id, signature, proposed_by, adopted_at
			FROM governance_rules WHERE raft_id = ?
		`, raftID)
		if err != nil {
			return fmt.Errorf("failed to query rules for raft %s: %w", raftID, err)
		}

		for ruleRows.Next() {
			var ruleID, raftIDCol, scope, body, proposedBy string
			var version int
			var timestamp int64
			var baseRuleID *string
			var signature []byte
			var adoptedAt *int64

			err := ruleRows.Scan(&ruleID, &raftIDCol, &scope, &version, &timestamp, &body, &baseRuleID, &signature, &proposedBy, &adoptedAt)
			if err != nil {
				ruleRows.Close()
				return fmt.Errorf("failed to scan rule: %w", err)
			}

			rule := &Rule{
				RuleID:     ruleID,
				RaftID:     raftIDCol,
				Scope:      scope,
				Version:    version,
				Timestamp:  time.Unix(timestamp, 0),
				Body:       body,
				Signature:  signature,
				ProposedBy: proposedBy,
			}

			if baseRuleID != nil {
				rule.BaseRuleID = *baseRuleID
			}

			if adoptedAt != nil {
				adopted := time.Unix(*adoptedAt, 0)
				rule.AdoptedAt = &adopted
			}

			raft.Rules[ruleID] = rule

			// Add to global rule registry if adopted
			if rule.AdoptedAt != nil {
				g.rules.mu.Lock()
				g.rules.rules[ruleID] = rule
				g.rules.active[scope] = rule
				g.rules.mu.Unlock()
			}
		}
		ruleRows.Close()

		// Add raft to registry
		g.rafts.mu.Lock()
		g.rafts.rafts[raftID] = raft
		g.rafts.mu.Unlock()
	}

	return nil
}

// getDB returns the database connection from the memory layer's vectorDB
func (g *Governance) getDB() *sql.DB {
	// The memory layer wraps the SQLiteVectorDB
	// We need to access it through the vectorDB interface
	vdb := g.memory.GetVectorDB()

	// Type assert to get the concrete SQLiteVectorDB type
	// This requires importing the vectordb package
	if sqliteVDB, ok := vdb.(interface{ GetDB() *sql.DB }); ok {
		return sqliteVDB.GetDB()
	}
	return nil
}
