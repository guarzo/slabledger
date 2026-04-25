/* global React */
const { useState } = React;

// ============== ICONS (inline, Feather/Lucide-style) ==============
const Icon = {
  Menu: () => <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg>,
  Close: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>,
  Caret: () => <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="6 9 12 15 18 9"/></svg>,
  Search: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>,
  Plus: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>,
  TrendUp: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M23 6l-9.5 9.5-5-5L1 18"/><polyline points="17 6 23 6 23 12"/></svg>,
  TrendDown: () => <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M23 18l-9.5-9.5-5 5L1 6"/><polyline points="17 18 23 18 23 12"/></svg>,
  Logout: () => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M17 16l4-4-4-4M21 12H7M13 17v1a3 3 0 01-3 3H6a3 3 0 01-3-3V6a3 3 0 013-3h4a3 3 0 013 3v1"/></svg>,
  GoogleG: () => (
    <svg width="18" height="18" viewBox="0 0 24 24">
      <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
      <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
      <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
      <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
    </svg>
  ),
};

// ============== PRIMITIVES ==============
function Button({ children, variant = "primary", size = "md", ...rest }) {
  return <button className={`sl-btn ${variant} ${size}`} {...rest}>{children}</button>;
}

function GradeBadge({ grader = "PSA", grade }) {
  const key = `${grader}-${grade}`;
  const styles = {
    "PSA-10": { bg: "rgba(251,191,36,0.14)", fg: "#fbbf24", bd: "rgba(251,191,36,0.3)" },
    "PSA-9":  { bg: "rgba(37,99,235,0.14)",  fg: "#60a5fa", bd: "rgba(37,99,235,0.3)" },
    "PSA-8":  { bg: "rgba(161,98,7,0.14)",   fg: "#fbbf24", bd: "rgba(161,98,7,0.3)" },
    "PSA-7":  { bg: "rgba(107,114,128,0.14)",fg: "#9ca3af", bd: "rgba(107,114,128,0.3)" },
    "BGS-10": { bg: "rgba(0,0,0,0.5)",       fg: "#e5e7eb", bd: "rgba(255,255,255,0.2)" },
    "CGC-9.5":{ bg: "rgba(245,158,11,0.14)", fg: "#f59e0b", bd: "rgba(245,158,11,0.3)" },
  };
  const s = styles[key] || styles["PSA-9"];
  return <span className="sl-grade" style={{ background: s.bg, color: s.fg, borderColor: s.bd }}>{grader} {grade}</span>;
}

function StatusPill({ tone = "info", children }) {
  const map = {
    success: { bg: "rgba(16,185,129,0.10)", fg: "#34d399", bd: "rgba(16,185,129,0.30)" },
    warning: { bg: "rgba(245,158,11,0.10)", fg: "#fbbf24", bd: "rgba(245,158,11,0.30)" },
    danger:  { bg: "rgba(239,68,68,0.10)",  fg: "#f87171", bd: "rgba(239,68,68,0.30)" },
    info:    { bg: "rgba(34,211,238,0.10)", fg: "#22d3ee", bd: "rgba(34,211,238,0.30)" },
    brand:   { bg: "rgba(99,102,241,0.10)", fg: "#a5b4fc", bd: "rgba(99,102,241,0.30)" },
    neutral: { bg: "rgba(107,114,128,0.10)",fg: "#9ca3af", bd: "rgba(107,114,128,0.30)" },
  };
  const s = map[tone] || map.info;
  return <span className="sl-pill" style={{ background: s.bg, color: s.fg, borderColor: s.bd }}>{children}</span>;
}

function RecommendationBadge({ tier = "BUY" }) {
  const map = {
    "MUST BUY":       { bg: "linear-gradient(135deg,#047857,#059669)", bd: "#065f46", glow: "0 4px 16px rgba(5,150,105,0.5)" },
    "STRONG BUY":     { bg: "linear-gradient(135deg,#059669,#10b981)", bd: "#047857" },
    "BUY":            { bg: "linear-gradient(135deg,#10b981,#34d399)", bd: "#059669" },
    "BUY WITH CAUTION": { bg: "linear-gradient(135deg,#f59e0b,#fbbf24)", bd: "#d97706" },
    "WATCH":          { bg: "linear-gradient(135deg,#6b7280,#9ca3af)", bd: "#4b5563" },
    "AVOID":          { bg: "linear-gradient(135deg,#dc2626,#ef4444)", bd: "#b91c1c" },
  };
  const s = map[tier] || map["BUY"];
  return <span className="sl-rec" style={{ background: s.bg, borderColor: s.bd, boxShadow: s.glow }}>{tier}</span>;
}

// ============== HEADER / NAV ==============
function Ticker() {
  return <span className="sl-ticker">PSA 10 +2.1% 24h</span>;
}

function Header({ route, setRoute, onSignOut }) {
  const [menuOpen, setMenuOpen] = useState(false);
  return (
    <header className="sl-header" data-screen-label="Header">
      <div className="sl-header-row">
        <a onClick={() => setRoute("dashboard")} style={{ display: "flex", alignItems: "center", gap: 10, cursor: "pointer", textDecoration: "none" }}>
          <img src="../../assets/slabledger-card-logo.png" alt="SlabLedger" style={{ width: 32, height: 32, borderRadius: 8 }}/>
          <div style={{ display: "flex", flexDirection: "column", lineHeight: 1 }}>
            <span style={{ fontSize: 15, fontWeight: 700, color: "var(--color-heading)" }}>SlabLedger</span>
            <span style={{ fontSize: 10, color: "var(--text-muted)", marginTop: 2, textTransform: "uppercase", letterSpacing: "0.08em" }}>Portfolio</span>
          </div>
        </a>
        <Navigation route={route} setRoute={setRoute} />
        <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 12 }}>
          <Ticker/>
          <button className="sl-btn ghost sm" style={{ padding: 6 }} aria-label="Search"><Icon.Search/></button>
          <div style={{ position: "relative" }}>
            <button className="sl-btn ghost sm" onClick={() => setMenuOpen(!menuOpen)} style={{ padding: 4, paddingRight: 8 }}>
              <span style={{ width: 26, height: 26, borderRadius: 999, background: "linear-gradient(135deg,#5a5de8,#34d399)", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 11, fontWeight: 700, color: "#fff" }}>JG</span>
              <Icon.Caret/>
            </button>
            {menuOpen && (
              <div style={{ position: "absolute", right: 0, top: "110%", background: "var(--surface-2)", border: "1px solid var(--surface-0)", borderRadius: 12, minWidth: 180, boxShadow: "0 10px 30px rgba(0,0,0,0.4)", padding: 6, zIndex: 60 }}>
                <div style={{ padding: "8px 12px", borderBottom: "1px solid rgba(255,255,255,0.06)" }}>
                  <div style={{ fontSize: 13, fontWeight: 600 }}>Jesse G.</div>
                  <div style={{ fontSize: 11, color: "var(--text-muted)" }}>jesse@cardyeti.co</div>
                </div>
                <button className="sl-btn ghost sm" style={{ width: "100%", justifyContent: "flex-start", marginTop: 4 }} onClick={onSignOut}><Icon.Logout/> Sign out</button>
              </div>
            )}
          </div>
        </div>
      </div>
    </header>
  );
}

function Navigation({ route, setRoute }) {
  const items = [
    { id: "dashboard", label: "Dashboard" },
    { id: "campaigns", label: "Campaigns" },
    { id: "inventory", label: "Inventory" },
    { id: "reprice", label: "Reprice" },
    { id: "insights", label: "Insights" },
    { id: "invoices", label: "Invoices" },
  ];
  return (
    <nav className="sl-nav">
      {items.map(it => (
        <a key={it.id} className={route === it.id ? "active" : ""} onClick={() => setRoute(it.id)}>{it.label}</a>
      ))}
    </nav>
  );
}

Object.assign(window, { Button, GradeBadge, StatusPill, RecommendationBadge, Header, Navigation, Icon });
