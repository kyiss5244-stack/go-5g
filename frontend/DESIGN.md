---
name: LiteCore
description: A calm, legible workspace for demonstrating 5G registration.
colors:
  lab-canvas: "#f5f7fa"
  paper: "#ffffff"
  network-ink: "#172b45"
  action-blue: "#245bc1"
  body-ink: "#202e40"
  secondary-ink: "#586779"
  divider: "#dce2ea"
  success: "#22734f"
  rejection: "#a63c24"
typography:
  display:
    fontFamily: "Geist, Microsoft YaHei, sans-serif"
    fontSize: "clamp(1.6rem, 2.6vw, 2.2rem)"
    fontWeight: 650
    lineHeight: 1.35
    letterSpacing: "-0.025em"
  body:
    fontFamily: "Geist, Microsoft YaHei, sans-serif"
    fontSize: "1rem"
    lineHeight: 1.6
  data:
    fontFamily: "Geist Mono, monospace"
    fontSize: "0.8125rem"
rounded:
  control: "0.5rem"
  surface: "1rem"
spacing:
  compact: "0.5rem"
  regular: "1rem"
  section: "2rem"
components:
  button-primary:
    backgroundColor: "{colors.action-blue}"
    textColor: "{colors.paper}"
    rounded: "{rounded.control}"
    height: "46px"
  input:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.body-ink}"
    rounded: "{rounded.control}"
    height: "44px"
---

# Design System: LiteCore

## Overview

**Creative North Star: “The Quiet Lab Bench”**

LiteCore is an operating surface, not a marketing page. It should feel like a well-kept daytime lab: the registration path is immediately visible, controls sit next to the system they affect, and historical evidence is separated from the current browser simulation.

The interface is restrained rather than sterile. Deep network ink anchors navigation, cool paper surfaces keep dense technical material readable, and action blue appears only where attention or progress is meaningful.

## Colors

Network ink establishes the product frame. Action blue carries selection and progress; green and rust are reserved for successful and rejected outcomes. Cool grays separate surfaces without decorative shadows or glass effects.

**The Evidence Color Rule.** Semantic colors always appear with text or an icon, never as the only indication of state.

## Typography

Geist is the working face for Chinese and Latin interface copy. Geist Mono is reserved for UE identifiers, ports, IP addresses, and measured values.

Hierarchy comes from size, weight, and whitespace. Labels remain sentence case; tracked uppercase and tiny technical captions are not part of this system.

## Layout

The main workspace is capped at 1344px with adaptive gutters. On desktop, the registration flow and its controls share one split surface; analytical evidence follows below. At 760px the split becomes a single column, navigation wraps, and records use two columns. Tables retain a deliberate horizontal scroll region on narrow screens.

Spacing follows a compact 8px-based rhythm, with larger separation between tasks than within a task.

## Elevation & Depth

The system is flat by default. Depth comes from tonal layers and single-pixel dividers; shadows are not used as decorative card chrome.

## Shapes

Major working surfaces use restrained 16px corners. Controls use 8px corners. Small status markers may be circular, while ordinary labels and containers never become pills by default.

## Components

Primary buttons are full-width only inside the registration form, with a 46px target and direct action wording. Inputs and selects are 44px high with visible labels, helper text, errors, focus rings, and disabled states.

The network path is the signature component: four semantic stages, connected in reading order, with progress, completion, and rejection shown through color, icon, and copy. Historical tables use tabular numerals and keep source context adjacent.

## Do's and Don'ts

### Do

- Do place the task and its immediate system response in the same visual field.
- Do label browser simulation and historical experiment output separately.
- Do use monospace only for technical identifiers and measurements.

### Don't

- Don't claim that gRPC services are online when the page has not checked them.
- Don't bring back neon glows, decorative grid backgrounds, or dense same-size metric cards.
- Don't invent live session counts, benchmark results, or timing claims.
