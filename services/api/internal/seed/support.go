package seed

import "fmt"

// seedSupport creates synthetic support tickets in various states.
func (s *seeder) seedSupport() error {
	if len(s.customerIDs) < 3 {
		return nil
	}
	tickets := []struct {
		cat, subject, body, status, priority string
		internal                             string
		reply                                string
	}{
		{"late_delivery", "Order arrived late", "My order BO-10004 took nearly an hour.",
			"resolved", "normal",
			"Rider allocation delay; offered coupon in demo.", "We are sorry about the delay. A ₹50 coupon (ORBIT50) is on your account (demo)."},
		{"missing_item", "Missing chutney packets", "Two green chutney packets were missing from my dosa order.",
			"resolved", "low", "", "Refund of ₹30 processed (simulated) for the missing items."},
		{"quality", "Food was cold", "The biryani arrived lukewarm.",
			"closed", "normal", "", "Thanks for the feedback — we shared it with the outlet."},
		{"cancellation_refund", "Refund not received", "I cancelled but haven't seen the refund yet.",
			"waiting_for_customer", "high", "Simulated refund shows processed in ledger.", "Could you confirm the last 4 digits of the account used (demo data only)?"},
		{"payment", "Charged but order rejected", "Payment captured but the restaurant rejected my order.",
			"assigned", "high", "Ledger check: refund queued automatically.", "Your refund is processed in this demo and visible in order payments."},
	}
	for i, t := range tickets {
		customer := s.customerIDs[i%len(s.customerIDs)]
		var assignee any
		if t.status != "open" {
			assignee = s.supportID
		}
		ticketID := s.mustInsert(`
			INSERT INTO support_tickets (reference, customer_id, category, subject, status, priority, assigned_to, created_at, updated_at, resolved_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7, now() - interval '9 days', now() - interval '2 days',
				CASE WHEN $5 IN ('resolved','closed') THEN now() - interval '2 days' END)
			RETURNING id`,
			fmt.Sprintf("SUP-%04d", 2001+i), customer, t.cat, t.subject, t.status, t.priority, assignee)
		if _, err := s.tx.Exec(s.ctx, `
			INSERT INTO support_messages (ticket_id, author_type, author_id, body, is_internal_note, created_at)
			VALUES ($1,'customer',$2,$3,false, now() - interval '9 days')`,
			ticketID, customer, t.body); err != nil {
			return err
		}
		if t.internal != "" {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO support_messages (ticket_id, author_type, author_id, body, is_internal_note, created_at)
				VALUES ($1,'agent',$2,$3,true, now() - interval '8 days')`,
				ticketID, s.supportID, t.internal); err != nil {
				return err
			}
		}
		if t.reply != "" {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO support_messages (ticket_id, author_type, author_id, body, is_internal_note, created_at)
				VALUES ($1,'agent',$2,$3,false, now() - interval '6 days')`,
				ticketID, s.supportID, t.reply); err != nil {
				return err
			}
		}
	}
	return nil
}
