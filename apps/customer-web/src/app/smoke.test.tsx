import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Logo, formatINR } from "@biteorbit/ui";

/**
 * Shell smoke test: proves the workspace React toolchain renders
 * shared BiteOrbit UI primitives inside this app.
 */
describe("customer app shell", () => {
  it("renders the BiteOrbit logo", () => {
    render(<Logo />);
    expect(screen.getByLabelText("BiteOrbit")).toBeTruthy();
  });

  it("uses shared money formatting", () => {
    expect(formatINR(29900)).toBe("₹299.00");
  });
});
