"use client";

import { apiRequest } from "@biteorbit/api-client";

export interface RiderProfile {
  rider_id: string;
  status: string;
  availability: string;
  vehicle_type: string;
  vehicle_number: string;
  name: string;
}

export function riderProfile() {
  return apiRequest<RiderProfile>("/api/v1/courier/me");
}

export function setAvailability(online: boolean) {
  return apiRequest<{ availability: string }>("/api/v1/courier/availability", {
    method: "POST",
    body: { online },
  });
}

export interface CourierOffer {
  assignment_id: string;
  order_id: string;
  reference: string;
  outlet_name: string;
  outlet_locality: string;
  drop_locality: string;
  distance_km: number;
  payout_total_paise: number;
  expires_in_sec: number;
  items_count: number;
}

export function listOffers() {
  return apiRequest<CourierOffer[]>("/api/v1/courier/offers");
}

export function acceptOffer(assignmentId: string) {
  return apiRequest<{ accepted: boolean; order_id: string }>(
    `/api/v1/courier/offers/${assignmentId}/accept`,
    { method: "POST" },
  );
}

export interface CourierAssignment {
  id: string;
  order_id: string;
  reference: string;
  status: string;
  outlet: { name: string; phone: string; locality: string };
  drop: {
    recipient_name: string;
    phone: string;
    line1: string;
    locality: string;
    city: string;
    instructions?: string | null;
  };
  distance_km: number;
  payout_total_paise: number;
  payout_breakdown: { base_paise: number; incentive_paise: number };
  items_count: number;
  payment_method: string;
  payment_status: string;
  allowed_next_states: string[];
}

export function getAssignment(id: string) {
  return apiRequest<CourierAssignment>(`/api/v1/courier/assignments/${id}`);
}

export function updateAssignmentState(
  id: string,
  state: string,
  opts: { otp?: string; reason?: string } = {},
) {
  return apiRequest<{ assignment_status: string; order_status: string }>(
    `/api/v1/courier/assignments/${id}/state`,
    { method: "POST", body: { state, otp: opts.otp, reason: opts.reason } },
  );
}

export interface CourierEarnings {
  days: Array<{
    day: string;
    deliveries: number;
    base_paise: number;
    incentive_paise: number;
    tip_paise: number;
    total_paise: number;
  }>;
  totals: { deliveries: number; total_paise: number; tip_paise: number; status: string };
  today_deliveries: number;
  availability: string;
  demo_note: string;
}

export function riderEarnings() {
  return apiRequest<CourierEarnings>("/api/v1/courier/earnings");
}
