import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  ArrowRight,
  CheckCircle2,
  Database,
  Download,
  ExternalLink,
  Gauge,
  Search,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  Zap,
} from "lucide-react";

const fmt = new Intl.NumberFormat("en-US");

function cn(...classes) {
  return classes.filter(Boolean).join(" ");
}

function fmtNumber(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "n/a";
  return fmt.format(value);
}

function fmtSeconds(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "n/a";
  return `${Number(value).toFixed(Number(value) < 1 ? 3 : 2)}s`;
}

function fmtRatio(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "not comparable";
  return `${Number(value).toFixed(2)}x`;
}

function shortSha(value) {
  return value ? value.slice(0, 12) : "not recorded";
}

function languageLabel(value) {
  if (value === "cpp") return "C++";
  if (value === "objc") return "Objective-C";
  if (value === "csharp") return "C#";
  return String(value || "").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function liveCoverageCount(data) {
  return data.liveSmokes.filter((row) => Number(row.coverage.ratio) >= 1).length;
}

function coreTotals(data) {
  const summaries = data.coreMatrix.map((row) => row.querySummary);
  return {
    atlasTokens: summaries.reduce((total, row) => total + row.atlasTokens, 0),
    graphifyTokens: summaries.reduce((total, row) => total + row.graphifyTokens, 0),
    atlasMs: summaries.reduce((total, row) => total + row.atlasMs, 0),
    graphifyMs: summaries.reduce((total, row) => total + row.graphifyMs, 0),
    equivalentRows: summaries.reduce((total, row) => total + row.equivalentRows, 0),
    rows: summaries.reduce((total, row) => total + row.rows, 0),
  };
}

function Button({ children, href, variant = "primary", download, className }) {
  const variantClass = variant === "primary"
    ? "bg-primary text-primary-foreground hover:bg-teal-700"
    : "border bg-white text-slate-900 hover:bg-slate-100";
  return (
    <a
      className={cn("inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-semibold transition", variantClass, className)}
      href={href}
      download={download}
    >
      {children}
    </a>
  );
}

function Card({ children, className, testId }) {
  return (
    <article
      data-testid={testId}
      className={cn("min-w-0 rounded-lg border bg-card text-card-foreground shadow-soft", className)}
    >
      {children}
    </article>
  );
}

function Badge({ children, tone = "neutral", className }) {
  const toneClass = {
    neutral: "border-slate-200 bg-slate-100 text-slate-700",
    success: "border-emerald-200 bg-emerald-50 text-emerald-700",
    warning: "border-amber-200 bg-amber-50 text-amber-800",
    primary: "border-teal-200 bg-teal-50 text-teal-800",
  }[tone];
  return (
    <span className={cn("inline-flex items-center rounded-full border px-2.5 py-1 text-xs font-semibold", toneClass, className)}>
      {children}
    </span>
  );
}

function SectionHeader({ eyebrow, title, children, actions }) {
  return (
    <div className="mb-6 flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
      <div className="max-w-3xl">
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h2 className="mt-2 text-2xl font-bold tracking-tight text-slate-950 md:text-3xl">{title}</h2>
        {children && <p className="mt-3 text-base text-muted-foreground">{children}</p>}
      </div>
      {actions && <div className="flex flex-wrap gap-2 md:justify-end">{actions}</div>}
    </div>
  );
}

function SourceLink({ label, href, download = false }) {
  return (
    <a
      data-source-artifact
      className="inline-flex items-center gap-1 rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-semibold text-slate-700 hover:bg-slate-100"
      href={href}
      target={download ? undefined : "_blank"}
      rel={download ? undefined : "noreferrer"}
      download={download}
    >
      {label}
    </a>
  );
}

function MetricCard({ icon: Icon, label, value, note }) {
  return (
    <Card className="p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
          <strong className="mt-2 block text-3xl font-bold tracking-tight text-slate-950">{value}</strong>
        </div>
        <span className="rounded-lg bg-teal-50 p-2 text-primary">
          <Icon className="h-5 w-5" aria-hidden="true" />
        </span>
      </div>
      <p className="mt-3 text-sm text-muted-foreground">{note}</p>
    </Card>
  );
}

function ComparisonBar({ label, atlasValue, graphifyValue, unit, ratio }) {
  const max = Math.max(atlasValue, graphifyValue, 1);
  const atlasWidth = (atlasValue / max) * 100;
  const graphifyWidth = (graphifyValue / max) * 100;
  return (
    <Card className="p-4">
      <div className="mb-4 flex items-center justify-between gap-3">
        <span className="text-sm font-bold text-muted-foreground">{label}</span>
        <strong className="text-2xl font-bold text-primary">{fmtRatio(ratio)}</strong>
      </div>
      <div className="space-y-3">
        <div className="grid grid-cols-[4rem_minmax(0,1fr)_5.5rem] items-center gap-3 text-xs font-semibold text-muted-foreground">
          <span>Atlas</span>
          <div className="h-3 overflow-hidden rounded-full bg-slate-200">
            <div className="h-full rounded-full bg-primary" style={{ width: `${atlasWidth}%` }} />
          </div>
          <span className="text-right">{fmtNumber(Math.round(atlasValue))} {unit}</span>
        </div>
        <div className="grid grid-cols-[4rem_minmax(0,1fr)_5.5rem] items-center gap-3 text-xs font-semibold text-muted-foreground">
          <span>graphify</span>
          <div className="h-3 overflow-hidden rounded-full bg-slate-200">
            <div className="h-full rounded-full bg-accent" style={{ width: `${graphifyWidth}%` }} />
          </div>
          <span className="text-right">{fmtNumber(Math.round(graphifyValue))} {unit}</span>
        </div>
      </div>
    </Card>
  );
}

function RatioMiniBar({ value, max = 42, tone = "primary" }) {
  const width = Math.max(0, Math.min(100, ((Number(value) || 0) / max) * 100));
  return (
    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-200">
      <div className={cn("h-full rounded-full", tone === "accent" ? "bg-accent" : "bg-primary")} style={{ width: `${width}%` }} />
    </div>
  );
}

function RowStatus({ summary }) {
  if (summary.equivalentRows === 0) return <Badge tone="warning">not comparable</Badge>;
  if (summary.pass5x) return <Badge tone="success">5x on comparable rows</Badge>;
  return <Badge tone="warning">below 5x threshold</Badge>;
}

function Header() {
  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur">
      <nav className="container-shell flex min-h-16 items-center justify-between gap-4" aria-label="Primary">
        <a className="flex min-w-0 items-center gap-3 text-slate-950 no-underline" href="#product">
          <span className="grid h-9 w-9 place-items-center rounded-lg bg-slate-950 text-sm font-black text-white">A</span>
          <span className="min-w-0">
            <span className="block text-base font-bold leading-tight">Atlas</span>
            <span className="hidden text-xs text-muted-foreground sm:block">AI review intelligence</span>
          </span>
        </a>
        <div className="hidden flex-wrap justify-end gap-1 md:flex">
          {[
            ["Product", "#product"],
            ["Proof", "#performance"],
            ["Trust", "#accuracy"],
            ["Install", "#install"],
            ["Data", "#downloads"],
            ["Details", "#matrix"],
          ].map(([label, href]) => (
            <a key={href} className="rounded-md px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-100" href={href}>{label}</a>
          ))}
        </div>
      </nav>
    </header>
  );
}

function Hero({ data }) {
  const totals = coreTotals(data);
  return (
    <section id="product" className="container-shell grid gap-6 py-8 lg:grid-cols-[minmax(0,1fr)_minmax(390px,0.78fr)] lg:py-12" aria-labelledby="page-title">
      <div className="flex flex-col justify-center">
        <Badge tone="primary" className="w-fit">Atlas for AI code review</Badge>
        <h1 id="page-title" className="mt-4 max-w-4xl text-4xl font-black leading-[1.05] tracking-tight text-slate-950 md:text-6xl">
          Cut AI review cost without losing the code context that matters
        </h1>
        <p className="mt-5 max-w-2xl text-lg text-muted-foreground">
          Atlas gives review agents the smallest useful picture of a code change. The result is lower token spend, faster answers, and a repeatable evidence pack leaders can inspect before rollout.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Button href="#performance">See the proof <ArrowRight className="h-4 w-4" /></Button>
          <Button href="data/benchmark-data.json" variant="secondary" download>Download evidence <Download className="h-4 w-4" /></Button>
          <Button href="#install" variant="secondary">Install locally <TerminalSquare className="h-4 w-4" /></Button>
          <Button href="https://github.com/aziron-ai/atlas/releases/latest" variant="secondary">GitHub releases <ExternalLink className="h-4 w-4" /></Button>
        </div>
      </div>
      <Card testId="product-hero-visual" className="flex min-w-0 flex-col gap-4 p-5">
        <div className="flex items-center justify-between gap-3 border-b pb-4">
          <div>
            <p className="text-sm font-semibold text-muted-foreground">Review agent input</p>
            <strong className="text-lg">Auditable proof</strong>
          </div>
          <Sparkles className="h-6 w-6 text-primary" aria-hidden="true" />
        </div>
        <ComparisonBar label="Token consumption" atlasValue={totals.atlasTokens} graphifyValue={totals.graphifyTokens} unit="tok" ratio={data.summary.core.tokenRatio} />
        <ComparisonBar label="Query latency" atlasValue={totals.atlasMs} graphifyValue={totals.graphifyMs} unit="ms" ratio={data.summary.core.latencyRatio} />
        <div className="mt-auto grid gap-2 sm:grid-cols-5">
          {["Private code stays local", "Relevant change context", "Lower token spend", "Faster review answers", "Evidence for audit"].map((item) => (
            <div key={item} className="grid min-h-14 place-items-center rounded-md border bg-slate-50 p-2 text-center text-xs font-bold text-slate-600">{item}</div>
          ))}
        </div>
      </Card>
    </section>
  );
}

function OutcomeStrip() {
  const items = [
    [Gauge, "Cost control", "Less context sent", "Atlas narrows review input to the code paths, symbols, and relationships that are actually relevant."],
    [Zap, "Cycle time", "Faster decisions", "Agents spend less time reading broad repository dumps and more time evaluating the change."],
    [ShieldCheck, "Governance", "Auditable evidence", "Every public number on this page is backed by downloadable benchmark JSON from live repositories."],
  ];
  return (
    <section className="container-shell pb-6" aria-labelledby="outcome-title">
      <div className="grid gap-4 rounded-xl border bg-white p-5 shadow-soft lg:grid-cols-[1.1fr_repeat(3,1fr)]">
        <div>
          <p className="eyebrow">What changes for the business</p>
          <h2 id="outcome-title" className="mt-2 text-2xl font-bold tracking-tight">AI review becomes cheaper, faster, and easier to govern</h2>
        </div>
        {items.map(([Icon, label, title, copy]) => (
          <div key={label} className="rounded-lg border bg-slate-50 p-4">
            <Icon className="h-5 w-5 text-primary" aria-hidden="true" />
            <p className="mt-3 text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
            <strong className="mt-1 block text-base">{title}</strong>
            <p className="mt-2 text-sm text-muted-foreground">{copy}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

function Performance({ data }) {
  const totals = coreTotals(data);
  const coverageAtNative = liveCoverageCount(data);
  const tokenSaved = totals.graphifyTokens - totals.atlasTokens;
  const latencySaved = totals.graphifyMs - totals.atlasMs;
  const metrics = [
    [Download, "Estimated token reduction", fmtRatio(data.summary.core.tokenRatio), `${fmtNumber(tokenSaved)} fewer response tokens across comparable core rows`],
    [Gauge, "Response speed", fmtRatio(data.summary.core.latencyRatio), `${fmtNumber(Math.round(latencySaved))} ms less aggregate response time in the core matrix`],
    [CheckCircle2, "Review coverage", `${coverageAtNative}/${data.summary.live.artifacts}`, "live checks at or above native definition coverage proxy"],
    [Database, "Evidence pack", data.sourceArtifacts.length, "downloadable benchmark JSON artifacts"],
  ];
  const languages = data.coreMatrix.map((row) => (
    <div key={row.language} className="grid items-center gap-3 text-sm sm:grid-cols-[7rem_minmax(0,1fr)_9rem]">
      <span className="font-semibold text-slate-700">{languageLabel(row.language)}</span>
      <div className="grid gap-1">
        <div className="h-2 rounded-full bg-slate-200">
          <div className="h-2 rounded-full bg-primary" style={{ width: `${Math.min(100, (row.querySummary.latencyRatio / 8) * 100)}%` }} />
        </div>
        <div className="h-2 rounded-full bg-slate-200">
          <div className="h-2 rounded-full bg-accent" style={{ width: `${Math.min(100, (row.querySummary.tokenRatio / 30) * 100)}%` }} />
        </div>
      </div>
      <span className="text-sm font-semibold text-muted-foreground">{fmtRatio(row.querySummary.latencyRatio)} / {fmtRatio(row.querySummary.tokenRatio)}</span>
    </div>
  ));

  return (
    <section id="performance" className="container-shell py-6" data-testid="executive-performance" aria-labelledby="performance-title">
      <Card className="p-5">
        <SectionHeader
          eyebrow="Executive proof"
          title="The benchmark focuses on the numbers buyers care about"
          actions={(
            <>
              <SourceLink label="Download derived JSON" href="data/benchmark-data.json" download />
              <SourceLink label="Download matrix JSON" href="data/raw/MATRIX_REPORT.json" download />
            </>
          )}
        >
          Token use drives AI cost. Latency drives developer wait time. Coverage and caveats show whether the evidence is trustworthy.
        </SectionHeader>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {metrics.map(([Icon, label, value, note]) => <MetricCard key={label} icon={Icon} label={label} value={value} note={note} />)}
        </div>
        <div id="comparison-bars" data-testid="token-latency-charts" className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
          <Card className="p-5">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <h3 className="text-lg font-bold">Core language ratios</h3>
                <p className="mt-1 text-sm text-muted-foreground">Latency and token ratios are shown only where both tools returned comparable query rows.</p>
              </div>
              <div className="flex gap-3 text-xs font-semibold text-muted-foreground">
                <span className="inline-flex items-center gap-1"><i className="h-2 w-5 rounded-full bg-primary" /> latency</span>
                <span className="inline-flex items-center gap-1"><i className="h-2 w-5 rounded-full bg-accent" /> tokens</span>
              </div>
            </div>
            <div className="mt-5 grid gap-4">{languages}</div>
          </Card>
          <Card className="p-5">
            <h3 className="text-lg font-bold">What this means</h3>
            <p className="mt-1 text-sm text-muted-foreground">Atlas reduces review-context cost without hiding non-comparable rows.</p>
            <div className="mt-5 grid gap-3">
              <div className="rounded-lg border bg-slate-50 p-4"><strong className="block text-3xl">{data.summary.core.equivalentRows}/{data.summary.core.queryRows}</strong><span className="text-sm text-muted-foreground">core comparable rows</span></div>
              <div className="rounded-lg border bg-slate-50 p-4"><strong className="block text-3xl">{data.summary.live.fiveXComparable}/{data.summary.live.withComparableRows}</strong><span className="text-sm text-muted-foreground">live rows above 5x on comparable rows</span></div>
              <div className="rounded-lg border bg-amber-50 p-4"><strong className="block text-3xl">{data.summary.saturation.noComparableRows}</strong><span className="text-sm text-amber-800">rows kept visible as not comparable</span></div>
            </div>
          </Card>
        </div>
      </Card>
    </section>
  );
}

function Accuracy({ data }) {
  const detector = data.provenance.graphify.detectorOnlyCodeExtensions.join(", ");
  const cards = [
    ["Grounded in real repositories", `${liveCoverageCount(data)}/${data.summary.live.artifacts}`, "live open-source checks at or above native or parser definition coverage."],
    ["Language coverage", `${data.summary.coverage.graphifyRows}/39`, `graphify language families audited against runtime discovery from ${data.provenance.graphify.version}.`],
    ["Known limits shown", detector, "detector-only rows are labeled instead of being presented as deterministic parity."],
    ["No inflated claims", data.summary.saturation.noComparableRows, "BYOND, ETS, and R have native coverage evidence but zero graphify-equivalent query rows."],
  ];
  return (
    <section id="accuracy" className="container-shell py-6" data-testid="accuracy-positioning" aria-labelledby="accuracy-title">
      <div className="grid gap-5 rounded-xl border bg-white p-5 shadow-soft lg:grid-cols-[0.8fr_1.2fr]">
        <div className="flex flex-col justify-center">
          <p className="eyebrow">Trust model</p>
          <h2 id="accuracy-title" className="mt-2 text-2xl font-bold tracking-tight md:text-3xl">Clear claims, visible limits, and raw data behind every chart</h2>
          <p className="mt-3 text-muted-foreground">Atlas is positioned with measured results, not blanket marketing claims. Comparable rows are shown separately from rows where another tool could not produce equivalent output.</p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {cards.map(([label, value, copy], index) => (
            <Card key={label} className={cn("p-5", index > 1 && "bg-amber-50")}>
              <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
              <strong className="mt-2 block text-2xl">{value}</strong>
              <p className="mt-2 text-sm text-muted-foreground">{copy}</p>
            </Card>
          ))}
        </div>
      </div>
    </section>
  );
}

function Downloads({ data }) {
  const cards = [
    ["Executive dataset", "benchmark-data.json", "Derived JSON used by every chart and table on this page.", "data/benchmark-data.json"],
    ["Core matrix", "MATRIX_REPORT.json", `${data.summary.core.languages} languages, ${data.summary.core.queryRows} query rows.`, "data/raw/MATRIX_REPORT.json"],
    ["Tool manifest", "MATRIX_TOOL_VERSIONS.json", `${data.provenance.tools.coreCount} core tools and ${data.provenance.tools.liveSmokeCount} live smoke tools.`, "data/raw/MATRIX_TOOL_VERSIONS.json"],
    ["Coverage discovery", "GRAPHIFY_LANGUAGE_DISCOVERY.json", `${data.provenance.graphify.dispatchCount} extractor entries and ${data.provenance.graphify.codeExtensionCount} code extensions.`, "data/raw/GRAPHIFY_LANGUAGE_DISCOVERY.json"],
  ];
  return (
    <section id="downloads" className="container-shell py-6" aria-labelledby="downloads-title">
      <Card className="grid gap-5 p-5 lg:grid-cols-[0.8fr_1.2fr]">
        <div>
          <p className="eyebrow">Downloadable evidence</p>
          <h2 id="downloads-title" className="mt-2 text-2xl font-bold tracking-tight md:text-3xl">Evidence package for architecture, procurement, and engineering review</h2>
          <p className="mt-3 text-muted-foreground">Download the executive dataset for internal dashboards, or audit the raw benchmark artifacts behind the public numbers.</p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {cards.map(([label, title, copy, href]) => (
            <a key={title} data-source-artifact className="rounded-lg border bg-slate-50 p-4 text-slate-950 no-underline hover:bg-slate-100" href={href} download>
              <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
              <strong className="mt-2 block break-words">{title}</strong>
              <p className="mt-2 text-sm text-muted-foreground">{copy}</p>
            </a>
          ))}
        </div>
      </Card>
    </section>
  );
}

function InstallGuide() {
  return (
    <section id="install" className="container-shell py-6" data-testid="install-guide" aria-labelledby="install-title">
      <Card className="p-5">
        <SectionHeader
          eyebrow="Pilot-ready install"
          title="Run Atlas locally with one binary and a local SQLite database"
          actions={(
            <>
              <a className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-semibold text-slate-700" href="https://github.com/aziron-ai/atlas/releases/latest">Latest release</a>
              <a className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-semibold text-slate-700" href="https://github.com/dominic097/homebrew-atlas">Homebrew tap</a>
              <a className="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-xs font-semibold text-slate-700" href="https://www.npmjs.com/package/@dominic097/atlas">npm package</a>
            </>
          )}
        >
          For pilots, the technical owner installs the atlas command, indexes a repository locally, and connects review agents through MCP. The default database is sqlite://./.atlas/atlas.db; no shared server is required.
        </SectionHeader>
        <div className="grid gap-4 lg:grid-cols-3">
          <InstallCard label="Homebrew" title="macOS cask" code={`brew install --cask dominic097/atlas/atlas\natlas version`} />
          <InstallCard label="npm" title="Node wrapper, atlas bin" code={`npm install -g @dominic097/atlas\natlas version`} />
          <InstallCard
            label="Linux packages"
            title="amd64 and arm64 release assets"
            code={`curl -LO https://github.com/aziron-ai/atlas/releases/download/v0.1.21/atlas_0.1.21_linux_amd64.tar.gz\ntar -xzf atlas_0.1.21_linux_amd64.tar.gz\nsudo install -m 0755 atlas /usr/local/bin/atlas\natlas version`}
            note="Release assets also include Linux .deb, .rpm, and .apk packages for amd64 and arm64."
          />
        </div>
        <div className="mt-4 grid gap-4 lg:grid-cols-3">
          <UsageStep number="1" title="Index a repository" code="atlas index . --reindex" copy="Builds the local symbol, call, route, and search graph into SQLite." />
          <UsageStep number="2" title="Retrieve review context" code={`atlas context --paths path/to/changed-file.go --query "review risk" --format json`} copy="Returns a compact context bundle around the changed files for a review agent." />
          <UsageStep number="3" title="Connect local assistant tools" code={`atlas mcp --transport http --http 127.0.0.1:8765\natlas install skill --agent codex\natlas install skill --agent claude`} copy="Lets local assistant tools ask Atlas for search, impact, and context without sending the repository graph to a hosted service." />
        </div>
        <div className="mt-4 grid items-center gap-4 rounded-lg border bg-slate-50 p-4 lg:grid-cols-[0.85fr_1.15fr]">
          <div>
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Benchmark data</p>
            <p className="mt-2 text-sm text-muted-foreground">Download the same JSON used by this page for internal review, dashboards, and procurement evidence.</p>
          </div>
          <pre className="code-block"><code>{`curl -LO https://aziron-ai.github.io/atlas/data/benchmark-data.json\ncurl -LO https://aziron-ai.github.io/atlas/data/raw/MATRIX_REPORT.json`}</code></pre>
        </div>
      </Card>
    </section>
  );
}

function InstallCard({ label, title, code, note }) {
  return (
    <Card className="p-4">
      <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
      <h3 className="mt-2 text-base font-bold">{title}</h3>
      <pre className="code-block mt-4"><code>{code}</code></pre>
      {note && <p className="mt-3 text-sm text-muted-foreground">{note}</p>}
    </Card>
  );
}

function UsageStep({ number, title, code, copy }) {
  return (
    <Card className="grid gap-3 p-4 sm:grid-cols-[auto_minmax(0,1fr)] lg:grid-cols-1 xl:grid-cols-[auto_minmax(0,1fr)]">
      <span className="grid h-8 w-8 place-items-center rounded-full bg-primary text-sm font-bold text-white">{number}</span>
      <div className="min-w-0">
        <h3 className="text-base font-bold">{title}</h3>
        <pre className="code-block mt-3"><code>{code}</code></pre>
        <p className="mt-3 text-sm text-muted-foreground">{copy}</p>
      </div>
    </Card>
  );
}

function KPIStrip({ data }) {
  const cards = [
    ["Core matrix", data.summary.core.languages, `${data.summary.core.equivalentRows}/${data.summary.core.queryRows} comparable query rows`],
    ["Core latency", fmtRatio(data.summary.core.latencyRatio), "graphify ms / Atlas ms, comparable rows"],
    ["Core tokens", fmtRatio(data.summary.core.tokenRatio), "graphify tokens / Atlas tokens, comparable rows"],
    ["Live checks", data.summary.live.artifacts, `${data.summary.live.withComparableRows} with comparable rows`],
    ["Language support", `${data.summary.coverage.deterministicRowsCovered}/${data.summary.coverage.graphifyRows}`, `${data.provenance.graphify.version}; ${data.summary.coverage.detectorOnlyRowsCovered} detector-only`],
  ];
  return (
    <section className="container-shell py-6" aria-label="Benchmark summary">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {cards.map(([label, value, note]) => (
          <Card key={label} className="p-4">
            <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
            <strong className="mt-2 block text-2xl">{value}</strong>
            <p className="mt-2 text-sm text-muted-foreground">{note}</p>
          </Card>
        ))}
      </div>
    </section>
  );
}

function Methodology({ data }) {
  const tools = data.provenance.tools.core
    .filter((tool) => ["atlas", "graphify", "scip-go", "scip-python", "scip-typescript", "scip-java", "gopls", "pyright", "tsc", "jdtls", "clangd"].includes(tool.tool));
  return (
    <section className="container-shell py-6" aria-labelledby="method-title">
      <Card className="p-5">
        <SectionHeader
          title="Evidence and methodology"
          actions={(
            <>
              <SourceLink label="derived JSON" href="data/benchmark-data.json" />
              <SourceLink label="matrix JSON" href="data/raw/MATRIX_REPORT.json" />
              <SourceLink label="saturation JSON" href="data/raw/SATURATION_REPORT.json" />
            </>
          )}
        >
          Ratios are shown only for equivalent query rows. Rows that cannot be compared stay visible instead of being folded into a marketing average.
        </SectionHeader>
        <div className="space-y-4 text-sm text-muted-foreground">
          <p data-testid="benchmark-source-root">Data source: {data.sourceLabel}. The page loads <span className="mono">data/benchmark-data.json</span>, generated from copied raw benchmark JSON artifacts in <span className="mono">data/raw/</span>.</p>
          <p>Tool manifest generated: <span className="mono">{data.provenance.toolManifestGeneratedAt}</span>. Platform: {data.provenance.platform.system} {data.provenance.platform.release} {data.provenance.platform.machine}; Python {data.provenance.platform.python}.</p>
        </div>
        <div className="table-wrap mt-5">
          <table className="data-table">
            <thead><tr><th>Tool</th><th>Status</th><th>Version</th></tr></thead>
            <tbody>{tools.map((tool) => <tr key={tool.tool}><td>{tool.tool}</td><td>{tool.status}</td><td className="mono">{tool.version || "n/a"}</td></tr>)}</tbody>
          </table>
        </div>
      </Card>
    </section>
  );
}

function CoreMatrix({ data }) {
  return (
    <section id="matrix" className="container-shell py-6" data-testid="core-matrix" aria-labelledby="matrix-title">
      <Card className="p-5">
        <SectionHeader
          title="Technical benchmark details"
          actions={<SourceLink label="MATRIX_REPORT.json" href="data/raw/MATRIX_REPORT.json" />}
        >
          Language-by-language results for Go, Python, JavaScript, TypeScript, Java, C, and C++ against graphify plus native baselines.
        </SectionHeader>
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Language</th><th>Repo</th><th>Atlas</th><th>graphify</th><th>Atlas graph</th><th>graphify graph</th><th>Atlas calls</th><th>graphify calls</th><th>Latency</th><th>Tokens</th><th>Queries</th><th>Source</th>
              </tr>
            </thead>
            <tbody>
              {data.coreMatrix.map((row) => {
                const atlas = row.atlas.metrics || {};
                const graphify = row.graphify.metrics || {};
                return (
                  <tr key={row.language} data-testid={`matrix-row-${row.language}`}>
                    <td>{languageLabel(row.language)}<br /><span className="mono text-xs text-muted-foreground">{row.language}</span></td>
                    <td><a className="text-accent hover:underline" href={row.repo} target="_blank" rel="noreferrer">{row.repo.replace("https://github.com/", "")}</a></td>
                    <td>{row.atlas.status}<br /><span className="text-muted-foreground">{fmtSeconds(atlas.cold_seconds ?? row.atlas.seconds)}</span></td>
                    <td>{row.graphify.status}<br /><span className="text-muted-foreground">{fmtSeconds(row.graphify.seconds)}</span></td>
                    <td>{fmtNumber(atlas.files)} files<br />{fmtNumber(atlas.symbols)} symbols<br />{fmtNumber(atlas.edges)} edges</td>
                    <td>{fmtNumber(graphify.nodes)} nodes<br />{fmtNumber(graphify.links)} links<br />{fmtNumber(graphify.calls)} calls</td>
                    <td>{fmtNumber(atlas.calls)} calls<br />{fmtNumber(atlas.internal_calls)} internal</td>
                    <td>{fmtNumber(graphify.extracted_calls)}/{fmtNumber(graphify.calls)}<br />{fmtNumber(graphify.extracted_pct)}% extracted</td>
                    <td className="min-w-32">{fmtRatio(row.querySummary.latencyRatio)}<RatioMiniBar value={row.querySummary.latencyRatio} max={10} /></td>
                    <td className="min-w-32">{fmtRatio(row.querySummary.tokenRatio)}<RatioMiniBar value={row.querySummary.tokenRatio} max={32} tone="accent" /></td>
                    <td>{row.querySummary.equivalentRows}/{row.querySummary.rows}<br /><RowStatus summary={row.querySummary} /></td>
                    <td><SourceLink label="JSON" href={row.artifact} /></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </Card>
    </section>
  );
}

function CoverageAudit({ data }) {
  return (
    <section id="coverage" className="container-shell py-6" data-testid="coverage-audit" aria-labelledby="coverage-title">
      <Card className="p-5">
        <SectionHeader title="Language coverage audit" actions={<SourceLink label="GRAPHIFY_LANGUAGE_DISCOVERY.json" href="data/raw/GRAPHIFY_LANGUAGE_DISCOVERY.json" />}>
          Runtime coverage evidence from extractor dispatch, code extensions, and detect smoke results.
        </SectionHeader>
        <div className="table-wrap">
          <table className="data-table">
            <thead><tr><th>Family</th><th>Extensions</th><th>graphify extractor</th><th>Support type</th><th>Atlas evidence</th><th>Source</th></tr></thead>
            <tbody>
              {data.coverageAudit.map((row) => (
                <tr key={`${row.family}-${row.extensions}`}>
                  <td>{row.family}</td>
                  <td className="mono">{row.extensions}</td>
                  <td>{row.graphifyExtractor}</td>
                  <td><Badge tone={row.supportType === "detector-only" ? "warning" : "success"}>{row.supportType === "detector-only" ? "detector-only" : "deterministic"}</Badge></td>
                  <td>{row.atlasStatus}</td>
                  <td><SourceLink label="source" href={row.artifact} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </section>
  );
}

function LiveChecks({ data }) {
  const [filter, setFilter] = useState("all");
  const [sort, setSort] = useState("language");
  const [search, setSearch] = useState("");
  const [detailLanguage, setDetailLanguage] = useState("rust");
  const rows = useMemo(() => {
    let next = [...data.liveSmokes];
    const term = search.trim().toLowerCase();
    if (filter === "comparable") next = next.filter((row) => row.querySummary.equivalentRows > 0);
    if (filter === "saturated") next = next.filter((row) => row.querySummary.equivalentRows === 0);
    if (filter === "detector") next = next.filter((row) => row.detectorOnly);
    if (term) next = next.filter((row) => [row.language, row.repo, row.commit, row.native.tool, row.artifact].some((value) => String(value || "").toLowerCase().includes(term)));
    next.sort((a, b) => {
      if (sort === "latency") return (b.querySummary.latencyRatio || 0) - (a.querySummary.latencyRatio || 0);
      if (sort === "tokens") return (b.querySummary.tokenRatio || 0) - (a.querySummary.tokenRatio || 0);
      if (sort === "coverage") return (b.coverage.ratio || 0) - (a.coverage.ratio || 0);
      if (sort === "cycles") return (b.optimization.cyclesRun || 0) - (a.optimization.cyclesRun || 0);
      return a.language.localeCompare(b.language);
    });
    return next;
  }, [data.liveSmokes, filter, sort, search]);
  const detail = data.liveSmokes.find((row) => row.language === detailLanguage) || data.liveSmokes[0];

  return (
    <section id="live" className="container-shell py-6" aria-labelledby="live-title">
      <Card className="p-5">
        <SectionHeader title="Live open-source repository checks" actions={<Badge tone="primary">{rows.length} rows</Badge>}>
          Each row uses a real open-source repo commit, Atlas indexing, graphify comparison, and the best available native or parser baseline.
        </SectionHeader>
        <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input id="live-search" className="min-h-10 w-full rounded-md border bg-white pl-9 pr-3 text-sm" type="search" placeholder="Search language, repo, commit, artifact" aria-label="Search live smokes" value={search} onChange={(event) => setSearch(event.target.value)} />
          </div>
          <div className="flex flex-wrap gap-2">
            <select id="live-filter" className="min-h-10 rounded-md border bg-white px-3 text-sm" aria-label="Filter live smokes" value={filter} onChange={(event) => setFilter(event.target.value)}>
              <option value="all">All rows</option>
              <option value="comparable">Comparable rows</option>
              <option value="saturated">No comparable rows</option>
              <option value="detector">Detector-only</option>
            </select>
            <select id="live-sort" className="min-h-10 rounded-md border bg-white px-3 text-sm" aria-label="Sort live smokes" value={sort} onChange={(event) => setSort(event.target.value)}>
              <option value="language">Language</option>
              <option value="latency">Latency ratio</option>
              <option value="tokens">Token ratio</option>
              <option value="coverage">Coverage ratio</option>
              <option value="cycles">Optimization cycles</option>
            </select>
          </div>
        </div>
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="table-wrap">
            <table className="data-table">
              <thead><tr><th>Language</th><th>Repo / commit</th><th>Native baseline</th><th>Coverage</th><th>Queries</th><th>Latency</th><th>Tokens</th><th>Cycles</th><th>Status</th><th>Evidence</th></tr></thead>
              <tbody id="live-body">
                {rows.map((row) => (
                  <tr key={row.language}>
                    <td>{languageLabel(row.language)}<br /><span className="mono text-xs text-muted-foreground">{row.language}</span></td>
                    <td><a className="text-accent hover:underline" href={row.repo} target="_blank" rel="noreferrer">{row.repo.replace("https://github.com/", "")}</a><br /><span className="mono text-xs text-muted-foreground">{shortSha(row.commit)}</span></td>
                    <td>{row.native.tool}<br /><Badge tone={row.native.ok ? "success" : "warning"}>{row.native.status}</Badge></td>
                    <td>{fmtRatio(row.coverage.ratio)}<br /><span className="text-muted-foreground">{fmtNumber(row.coverage.atlasDefinitions)} / {fmtNumber(row.coverage.nativeDefinitions)} defs</span></td>
                    <td>{row.querySummary.equivalentRows}/{row.querySummary.rows}<br />{row.querySummary.graphifyMissing ? <span className="text-muted-foreground">{row.querySummary.graphifyMissing} graphify missing</span> : null}</td>
                    <td>{fmtRatio(row.querySummary.latencyRatio)}<RatioMiniBar value={row.querySummary.latencyRatio} max={10} /></td>
                    <td>{fmtRatio(row.querySummary.tokenRatio)}<RatioMiniBar value={row.querySummary.tokenRatio} max={42} tone="accent" /></td>
                    <td>{row.optimization.cyclesRun ?? "n/a"}</td>
                    <td>{row.detectorOnly ? <Badge tone="warning">detector-only</Badge> : <RowStatus summary={row.querySummary} />}</td>
                    <td><button className="rounded-md border bg-white px-3 py-2 text-sm font-semibold hover:bg-slate-50" type="button" onClick={() => setDetailLanguage(row.language)}>Inspect</button><br /><SourceLink label="raw" href={row.artifact} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <DetailDrawer row={detail} />
        </div>
      </Card>
    </section>
  );
}

function DetailDrawer({ row }) {
  return (
    <aside className="min-w-0 rounded-lg border bg-white p-4 shadow-soft xl:sticky xl:top-20 xl:max-h-[calc(100vh-6rem)] xl:overflow-auto" aria-label="Selected language evidence">
      <h3 className="text-lg font-bold">{languageLabel(row.language)} evidence</h3>
      <p className="mt-2 break-words text-sm"><a className="text-accent hover:underline" href={row.repo} target="_blank" rel="noreferrer">{row.repo}</a></p>
      <p className="mono mt-2 break-words text-xs text-muted-foreground">{row.commit}</p>
      <p className="mt-4 text-sm">Native baseline: <strong>{row.native.tool}</strong> <Badge tone={row.native.ok ? "success" : "warning"}>{row.native.status}</Badge></p>
      <p className="mt-3 text-sm text-muted-foreground">Coverage proxy: <strong className="text-slate-950">{fmtRatio(row.coverage.ratio)}</strong>. Comparable query rows: <strong className="text-slate-950">{row.querySummary.equivalentRows}/{row.querySummary.rows}</strong>.</p>
      <p className="mt-3"><SourceLink label="Raw JSON artifact" href={row.artifact} /></p>
      <div className="table-wrap mt-4">
        <table className="data-table min-w-[620px]">
          <thead><tr><th>Query</th><th>Atlas</th><th>graphify</th><th>Atlas tok</th><th>graphify tok</th><th>Status</th></tr></thead>
          <tbody>
            {row.queries.map((query) => (
              <tr key={`${row.language}-${query.symbol}`}>
                <td>{query.symbol}</td>
                <td>{fmtSeconds((query.atlasMs || 0) / 1000)}</td>
                <td>{fmtSeconds((query.graphifyMs || 0) / 1000)}</td>
                <td>{fmtNumber(query.atlasTokens)}</td>
                <td>{fmtNumber(query.graphifyTokens)}</td>
                <td><Badge tone={query.graphifyMissing ? "warning" : "success"}>{query.graphifyMissing ? "graphify missing" : "comparable"}</Badge></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <h3 className="mt-5 text-base font-bold">Optimization note</h3>
      <p className="mt-2 text-sm text-muted-foreground">{row.optimization.stopReason || "No optimization note recorded."}</p>
    </aside>
  );
}

function Saturation({ data }) {
  return (
    <section id="saturation" className="container-shell py-6" data-testid="saturation-evidence" aria-labelledby="saturation-title">
      <Card className="p-5">
        <SectionHeader title="Where comparison was not possible" actions={<SourceLink label="SATURATION_REPORT.json" href="data/raw/SATURATION_REPORT.json" />}>
          These rows have repeated live attempts and native coverage evidence, but graphify produced zero equivalent query rows.
        </SectionHeader>
        <div className="table-wrap">
          <table className="data-table">
            <thead><tr><th>Language</th><th>Status</th><th>Iterations</th><th>Equivalent rows by pass</th><th>graphify missing by pass</th><th>Ratio</th><th>Source</th></tr></thead>
            <tbody>
              {data.saturation.map((row) => (
                <tr key={row.language}>
                  <td>{languageLabel(row.language)}</td>
                  <td>{row.status}</td>
                  <td>{row.iterationsRun}/{row.iterationsRequested}</td>
                  <td>{row.iterations.map((iteration) => iteration.equivalentRows).join(", ")}</td>
                  <td>{row.iterations.map((iteration) => iteration.graphifyMissing).join(", ")}</td>
                  <td>not comparable</td>
                  <td><SourceLink label="saturation JSON" href={row.artifact} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </section>
  );
}

function EvidenceArtifacts({ data }) {
  const important = ["MATRIX_REPORT.json", "MATRIX_TOOL_VERSIONS.json", "GRAPHIFY_LANGUAGE_DISCOVERY.json", "SATURATION_REPORT.json", "REPORT.json"];
  const artifacts = data.sourceArtifacts
    .filter((artifact) => important.includes(artifact.name) || /^LIVE_(RUST|BYOND|ETS|R|JAVA|PYTHON|APEX)_SMOKE/.test(artifact.name))
    .slice(0, 18);
  return (
    <section id="evidence" className="container-shell py-6" aria-labelledby="evidence-title">
      <Card className="p-5">
        <SectionHeader title="Raw JSON artifacts">
          The dashboard is generated from these committed benchmark artifacts. Generated at <span className="mono">{data.generatedAt}</span>.
        </SectionHeader>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {artifacts.map((artifact) => (
            <div key={artifact.path} className="min-w-0 rounded-lg border bg-white p-4">
              <a className="break-words text-sm font-semibold text-accent hover:underline" href={artifact.path} data-source-artifact target="_blank" rel="noreferrer">{artifact.name}</a>
              <div className="mt-2 text-sm text-muted-foreground">{fmtNumber(artifact.bytes)} bytes</div>
              <div className="mono mt-1 text-xs text-muted-foreground">{artifact.sha256.slice(0, 16)}</div>
            </div>
          ))}
        </div>
      </Card>
    </section>
  );
}

function Caveats({ data }) {
  return (
    <section className="container-shell py-6" aria-labelledby="caveats-title">
      <div className="rounded-lg border border-amber-200 bg-amber-50 p-5">
        <h2 id="caveats-title" className="text-xl font-bold">Caveats</h2>
        <ul className="mt-3 list-disc space-y-2 pl-5 text-sm text-amber-900">
          {data.caveats.map((item) => <li key={item}>{item}</li>)}
        </ul>
      </div>
    </section>
  );
}

function App() {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetch("data/benchmark-data.json", { cache: "no-store" })
      .then((response) => {
        if (!response.ok) throw new Error(`Unable to load benchmark data: ${response.status}`);
        return response.json();
      })
      .then(setData)
      .catch((err) => {
        console.error(err);
        setError(err);
      });
  }, []);

  if (error) {
    return (
      <main className="container-shell py-12">
        <div className="rounded-lg border border-red-200 bg-red-50 p-5 text-red-900">{error.message}</div>
      </main>
    );
  }

  if (!data) {
    return (
      <main className="container-shell py-12">
        <div className="rounded-lg border bg-white p-5 shadow-soft">Loading benchmark evidence...</div>
      </main>
    );
  }

  return (
    <>
      <a className="skip-link" href="#main">Skip to content</a>
      <Header />
      <main id="main">
        <Hero data={data} />
        <OutcomeStrip />
        <Performance data={data} />
        <Accuracy data={data} />
        <Downloads data={data} />
        <InstallGuide />
        <KPIStrip data={data} />
        <Methodology data={data} />
        <CoreMatrix data={data} />
        <CoverageAudit data={data} />
        <LiveChecks data={data} />
        <Saturation data={data} />
        <EvidenceArtifacts data={data} />
        <Caveats data={data} />
      </main>
      <footer className="container-shell py-8 text-sm text-muted-foreground">
        Atlas is published as a local binary and release asset. This page is static and data-driven from raw JSON evidence.
      </footer>
    </>
  );
}

createRoot(document.getElementById("root")).render(<App />);
