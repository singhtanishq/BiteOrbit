"use client";

import { apiRequest } from "@biteorbit/api-client";

export interface AdminOverview {
  orders_total: number;
  orders_by_status: Record<string, number>;
  gov_paise: number;
  platform_fees_paise: number;
  delivery_fees_paise: number;
  refunds_paise: number;
  coupon_cost_paise: number;
  active_outlets: number;
  pending_applications: number;
  riders_online: number;
  riders_total: number;
  users_total: number;
  tickets_open: number;
  data_as_of: string;
  demo_note: string;
}

export function adminOverview(days: number) {
  return apiRequest<AdminOverview>("/api/v1/admin/overview", { query: { days } });
}

export interface AdminOrderRow {
  id: string;
  reference: string;
  status: string;
  grand_total_paise: number;
  placed_at: string;
  outlet_name: string;
  customer_email: string;
  payment_status: string;
}

export function adminOrders(query: string, status: string) {
  return apiRequest<AdminOrderRow[]>("/api/v1/admin/orders", {
    query: { query: query || undefined, status: status || undefined, limit: 20 },
  });
}

export function adminCancelOrder(orderId: string, reason: string) {
  return apiRequest<{ status: string; audited: boolean }>(`/api/v1/admin/orders/${orderId}/cancel`, {
    method: "POST",
    body: { reason },
  });
}

export interface AdminUserRow {
  id: string;
  name: string;
  email: string;
  status: string;
  created_at: string;
  roles: string[];
  orders: number;
}

export function adminUsers(role: string, query: string) {
  return apiRequest<AdminUserRow[]>("/api/v1/admin/users", {
    query: { role: role || undefined, query: query || undefined },
  });
}

export function adminSetUserStatus(id: string, action: "suspend" | "activate", reason: string) {
  return apiRequest<{ status: string }>(`/api/v1/admin/users/${id}/${action}`, {
    method: "POST",
    body: { reason },
  });
}

export interface AdminOrgRow {
  id: string;
  name: string;
  status: string;
  contact_email: string;
  outlets: number;
  application_status: string;
}

export function adminRestaurants() {
  return apiRequest<AdminOrgRow[]>("/api/v1/admin/restaurants");
}

export interface AdminApplicationRow {
  id: string;
  status: string;
  organisation: string;
  applicant_note: string;
  created_at: string;
  reviewer_email: string;
  decision_note: string;
}

export function adminApplications(status?: string) {
  return apiRequest<AdminApplicationRow[]>("/api/v1/admin/applications", {
    query: { status: status || undefined },
  });
}

export function adminReviewApplication(id: string, decision: string, note: string) {
  return apiRequest<{ status: string; simulated: boolean }>(`/api/v1/admin/applications/${id}/review`, {
    method: "POST",
    body: { decision, note },
  });
}

export interface AdminRiderRow {
  id: string;
  name: string;
  email: string;
  status: string;
  availability: string;
  vehicle: string;
  vehicle_number: string;
  deliveries: number;
}

export function adminRiders() {
  return apiRequest<AdminRiderRow[]>("/api/v1/admin/riders");
}

export function adminRiderAction(id: string, action: "approve" | "suspend", reason: string) {
  return apiRequest<{ status: string }>(`/api/v1/admin/riders/${id}/${action}`, {
    method: "POST",
    body: { reason },
  });
}

export interface AdminCouponRow {
  id: string;
  code: string;
  title: string;
  discount_type: string;
  fixed_paise?: number | null;
  percent_bp?: number | null;
  min_order_paise: number;
  is_active: boolean;
  redeemed_count: number;
  usage_limit_per_user: number;
  funding_party: string;
  new_users_only: boolean;
}

export function adminCoupons() {
  return apiRequest<AdminCouponRow[]>("/api/v1/admin/coupons");
}

export function adminCreateCoupon(body: Record<string, unknown>) {
  return apiRequest<{ id: string; code: string }>("/api/v1/admin/coupons", { method: "POST", body });
}

export function adminUpdateCoupon(id: string, isActive: boolean, reason: string) {
  return apiRequest<{ is_active: boolean }>(`/api/v1/admin/coupons/${id}`, {
    method: "PATCH",
    body: { is_active: isActive, reason },
  });
}

export interface AuditRow {
  id: number;
  actor_type: string;
  actor_id: string;
  action: string;
  target_type: string;
  target_id: string;
  before_state?: string | null;
  after_state?: string | null;
  reason?: string | null;
  created_at: string;
}

export function adminAuditLogs(targetType?: string, targetID?: string) {
  return apiRequest<AuditRow[]>("/api/v1/admin/audit-logs", {
    query: { target_type: targetType || undefined, target_id: targetID || undefined },
  });
}

export interface AdminFlag {
  key: string;
  enabled: boolean;
  description: string;
}

export function adminFlags() {
  return apiRequest<AdminFlag[]>("/api/v1/admin/feature-flags");
}

export function adminUpdateFlag(key: string, enabled: boolean, reason: string) {
  return apiRequest<{ key: string; enabled: boolean }>(`/api/v1/admin/feature-flags/${key}`, {
    method: "PATCH",
    body: { enabled, reason },
  });
}

export interface AdminTicketRow {
  id: string;
  reference: string;
  category: string;
  subject: string;
  status: string;
  priority: string;
  customer_email: string;
  created_at: string;
  updated_at: string;
}

export function adminTickets(status?: string) {
  return apiRequest<AdminTicketRow[]>("/api/v1/admin/support/tickets", {
    query: { status: status || undefined },
  });
}

export interface AdminTicketThread {
  ticket: AdminTicketRow & { order_id?: string | null };
  messages: Array<{ id: number; author_type: string; body: string; is_internal_note: boolean; created_at: string }>;
}

export function adminTicketDetail(id: string) {
  return apiRequest<AdminTicketThread>(`/api/v1/admin/support/tickets/${id}`);
}

export function adminReplyTicket(id: string, body: string, internalNote: boolean) {
  return apiRequest<{ replied: boolean }>(`/api/v1/admin/support/tickets/${id}/reply`, {
    method: "POST",
    body: { body, internal_note: internalNote },
  });
}

export function adminUpdateTicket(id: string, body: { status?: string; priority?: string }) {
  return apiRequest<{ updated: boolean }>(`/api/v1/admin/support/tickets/${id}/status`, {
    method: "POST",
    body,
  });
}
