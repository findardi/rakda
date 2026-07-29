---
name: vdr-competitor-scan
description: Research how the benchmark virtual data room products (Ellty, iDeals, Ansarada, Datasite) handle a given feature or capability, and produce a comparison table of their approaches and common use cases. Use this skill whenever the user is scoping, naming, designing, or comparing any VDR feature — permissions, Q&A, redaction, watermarking, audit trails, analytics, NDA gating, folder structure, bidder tracking, and so on — and also whenever they ask "how do competitors do X", "what's standard for Y", "should we build Z", or ask to research or benchmark a feature. Trigger even when the user does not name the competitors explicitly; if the task involves deciding what a VDR feature should do, this skill applies.
---

# VDR competitor scan

Produce a grounded comparison of how four benchmark products handle a
capability, so feature decisions are made against what exists rather than
from memory.

## Before starting

Confirm two things with the user if they are not already clear:

1. **The capability being scanned.** "Permissions" is too broad to be useful.
   "Per-document download restrictions for external users" is scannable.
2. **The decision behind it.** Scoping a new feature, naming an existing one,
   and checking parity before a demo need different outputs. Ask which.

If the user gives a broad area, propose a narrower scan and let them confirm
before spending time on it.

## Scan procedure

Read `references/products.md` first — it holds the URLs, positioning, and known
differentiators for each product.

For each of the four products, in this order:

1. Start from the product's own documentation, help centre, or feature pages.
   These are the primary source.
2. Record what the capability is **called** in that product. Naming is often
   the most useful output of a scan — it reveals the vocabulary buyers already
   know.
3. Record what it actually **does**, in one or two sentences, concretely enough
   that someone could build against it.
4. Record the **use case** the product attaches to it — who they say it is for
   and in which deal stage.
5. Note anything the product is conspicuously **missing** relative to the
   others.

Fall back to review sites (G2, Capterra) only for things vendor documentation
will not state plainly: pricing complaints, usability friction, support
quality. Mark these clearly as third-party. Never use a review site for a
factual claim about what a feature does.

## Handling gaps

Vendor documentation for enterprise VDRs is often thin because the sales
motion runs through demos rather than public docs. When you cannot verify
something:

- Say so explicitly. Write "not documented publicly" rather than inferring
  from the product's general positioning.
- Do not fill the gap from training data. These products ship changes
  frequently and remembered feature lists go stale.
- One verified gap is more useful than four confident guesses.

## Output

Default to a markdown table in the conversation:

| | Ellty | iDeals | Ansarada | Datasite |
|---|---|---|---|---|
| Name for it | | | | |
| What it does | | | | |
| Stated use case | | | | |
| Notable limits | | | | |

Follow the table with three short sections:

**Convergence** — where all four agree. This is table stakes; the feature
probably needs to exist and should use the shared vocabulary.

**Divergence** — where they differ meaningfully. Present the options and the
tradeoff each represents. Do not silently pick one.

**Out of scope** — competitor capabilities that exist but do not fit this
project. State why. Parity is not the goal, and naming the exclusions is what
keeps a scan from turning into a backlog.

Produce an .xlsx instead only if the user asks for one, or if the scan covers
more than about six capabilities at once.

## Rules

- Cite the source for every row. A claim without a source is a guess.
- Compare like with like. Ellty sits a tier below the other three on price and
  target buyer; a feature it lacks may be a segment decision rather than a gap.
  Flag this rather than scoring it as behind.
- Report what the products do, not what this project should do. The
  recommendation is the user's call; the scan informs it.
- If two sources conflict, prefer the vendor's own current documentation and
  note the conflict.