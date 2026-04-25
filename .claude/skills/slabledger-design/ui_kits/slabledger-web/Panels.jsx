/* global React */
const { useState } = React;

// ============== HERO STATS ==============
function HeroStatsBar({ roi = "+18.4%", stats }) {
  const defaults = [
    { label: "Deployed", value: "$62,480" },
    { label: "Recovered", value: "$73,990" },
    { label: "At Risk", value: "$4,220", tone: "warn" },
    { label: "Wks to Cover", value: "3.1" },
    { label: "Outstanding", value: "$12,840" },
    { label: "30d Recovery", value: "$18,240 ↗" },
  ];
  const data = stats || defaults;
  return (
    <div className="sl-hero">
      <div>
        <div className="roi-label">Realized ROI</div>
        <div className="roi-value">{roi}</div>
      </div>
      <div className="stats">
        {data.map((s, i) => (
          <div key={i}>
            <div className="s-label">{s.label}</div>
            <div className="s-val" style={s.tone === "warn" ? { color: "var(--warning)" } : s.tone === "neg" ? { color: "var(--danger)" } : null}>{s.value}</div>
          </div>
        ))}
        <div style={{ alignSelf: "center" }}><StatusPill tone="warning">3 unpaid invoices →</StatusPill></div>
      </div>
    </div>
  );
}

// ============== LEDGER TABLE ==============
function LedgerTable({ rows }) {
  const defaults = [
    { card: "Charizard VMAX · Shining Fates", grader: "PSA", grade: "10", cost: "$280.00", sold: "$362.50", pl: "+$82.50", pos: true, channel: "eBay" },
    { card: "Pikachu V · Celebrations", grader: "PSA", grade: "9", cost: "$42.00", sold: "$58.00", pl: "+$16.00", pos: true, channel: "TCGPlayer" },
    { card: "Umbreon VMAX · Evolving Skies", grader: "PSA", grade: "8", cost: "$1,420.00", sold: "$1,280.00", pl: "−$140.00", pos: false, channel: "eBay" },
    { card: "Mewtwo GX · Shining Legends", grader: "BGS", grade: "10", cost: "$640.00", sold: "$890.00", pl: "+$250.00", pos: true, channel: "Local" },
    { card: "Lugia V Alt Art · Silver Tempest", grader: "PSA", grade: "10", cost: "$380.00", sold: "$522.00", pl: "+$142.00", pos: true, channel: "Website" },
    { card: "Gengar VMAX · Fusion Strike", grader: "CGC", grade: "9.5", cost: "$210.00", sold: "$188.00", pl: "−$22.00", pos: false, channel: "Card show" },
  ];
  const data = rows || defaults;
  return (
    <table className="sl-table">
      <thead>
        <tr>
          <th>Card</th>
          <th>Grade</th>
          <th>Channel</th>
          <th className="num">Purchase</th>
          <th className="num">Sold</th>
          <th className="num">P/L</th>
        </tr>
      </thead>
      <tbody>
        {data.map((r, i) => (
          <tr key={i} className={r.pos ? "pos" : "neg"}>
            <td>{r.card}</td>
            <td><GradeBadge grader={r.grader} grade={r.grade}/></td>
            <td style={{ color: "var(--text-muted)" }}>{r.channel}</td>
            <td className="num">{r.cost}</td>
            <td className="num">{r.sold}</td>
            <td className={`num ${r.pos ? "sl-pos" : "sl-neg"}`}>{r.pl}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// ============== INVENTORY CARDS ==============
function InventoryRow({ name, set, grader, grade, deployed, market, direction, daysHeld, rec }) {
  const dirColor = { rising: "var(--success)", falling: "var(--danger)", stable: "var(--text-muted)" }[direction];
  const DirIcon = direction === "rising" ? Icon.TrendUp : direction === "falling" ? Icon.TrendDown : null;
  return (
    <div className="sl-card interactive" style={{ display: "grid", gridTemplateColumns: "1.5fr 100px 120px 120px 140px 110px", alignItems: "center", gap: 12, padding: "14px 18px" }}>
      <div>
        <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-heading)" }}>{name}</div>
        <div style={{ fontSize: 11, color: "var(--text-muted)", marginTop: 2 }}>{set}</div>
      </div>
      <GradeBadge grader={grader} grade={grade}/>
      <div style={{ fontSize: 13, fontVariantNumeric: "tabular-nums" }}>{deployed}</div>
      <div style={{ fontSize: 13, fontVariantNumeric: "tabular-nums", display: "flex", alignItems: "center", gap: 6, color: dirColor }}>
        {DirIcon && <DirIcon/>} {market}
      </div>
      <div style={{ fontSize: 12, color: daysHeld > 60 ? "var(--warning)" : "var(--text-muted)" }}>{daysHeld}d held</div>
      <RecommendationBadge tier={rec}/>
    </div>
  );
}

// ============== EMPTY STATE ==============
function EmptyState({ icon = "📊", title, description, steps }) {
  return (
    <div className="sl-card" style={{ textAlign: "center", padding: 40 }}>
      <div style={{ fontSize: 40, marginBottom: 12 }}>{icon}</div>
      <div style={{ fontSize: 16, fontWeight: 600, color: "var(--color-heading)", marginBottom: 6 }}>{title}</div>
      <div style={{ fontSize: 13, color: "var(--text-muted)", marginBottom: 20 }}>{description}</div>
      {steps && (
        <ol style={{ textAlign: "left", maxWidth: 340, margin: "0 auto", paddingLeft: 20, fontSize: 13, color: "var(--text-muted)", lineHeight: 1.7 }}>
          {steps.map((s, i) => <li key={i}>{s}</li>)}
        </ol>
      )}
    </div>
  );
}

// ============== LOGIN CARD ==============
function LoginCard({ onSignIn }) {
  const [loading, setLoading] = useState(false);
  const handle = () => {
    setLoading(true);
    setTimeout(() => { setLoading(false); onSignIn(); }, 600);
  };
  return (
    <div className="sl-card glass" style={{ maxWidth: 420, width: "100%", padding: 40, position: "relative", zIndex: 1, borderRadius: 22 }}>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 10, marginBottom: 28 }}>
        <img src="../../assets/slabledger-card-logo.png" alt="" style={{ width: 80, height: 80, borderRadius: 16, boxShadow: "0 8px 24px rgba(0,0,0,0.4), 0 0 20px rgba(99,102,241,0.3)" }}/>
        <div style={{ fontSize: 28, fontWeight: 800, letterSpacing: "-0.02em", color: "var(--color-heading)" }}>SlabLedger</div>
        <div style={{ fontSize: 12, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: "0.12em", fontWeight: 600 }}>Graded Card Portfolio Tracker</div>
      </div>
      <p style={{ fontSize: 13, color: "var(--text-muted)", lineHeight: 1.6, textAlign: "center", margin: "0 0 28px" }}>
        Track PSA Direct Buy campaigns, manage card inventory across multiple sell channels, and analyze profitability with market direction signals.
      </p>
      <button className="sl-google-btn" onClick={handle} disabled={loading}>
        <Icon.GoogleG/> {loading ? "Signing in…" : "Sign in with Google"}
      </button>
      <div style={{ display: "flex", justifyContent: "space-around", marginTop: 28, paddingTop: 20, borderTop: "1px solid rgba(255,255,255,0.05)", fontSize: 11, color: "var(--text-muted)" }}>
        <div>📊 Campaign tracking</div>
        <div>💰 P&L analytics</div>
        <div>🔍 Price lookup</div>
      </div>
    </div>
  );
}

Object.assign(window, { HeroStatsBar, LedgerTable, InventoryRow, EmptyState, LoginCard });
