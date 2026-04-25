/* global React, HeroStatsBar, LedgerTable, InventoryRow, EmptyState, LoginCard, Header, Button, StatusPill, GradeBadge, RecommendationBadge, Icon */
const { useState } = React;

function LoginPage({ onSignIn }) {
  return (
    <div className="sl-login-page" data-screen-label="01 Login">
      <div className="sl-orb primary"></div>
      <div className="sl-orb secondary"></div>
      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 20, position: "relative", zIndex: 1 }}>
        <LoginCard onSignIn={onSignIn}/>
        <img src="../../assets/card-yeti-logo.png" alt="Card Yeti" style={{ width: 140, opacity: 0.85, filter: "drop-shadow(0 4px 12px rgba(0,0,0,0.4))" }}/>
        <div style={{ fontSize: 10, color: "var(--text-subtle)", letterSpacing: "0.08em", textTransform: "uppercase" }}>A Card Yeti tool · v2.4.1</div>
      </div>
    </div>
  );
}

function DashboardPage() {
  return (
    <div data-screen-label="02 Dashboard">
      <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", marginBottom: 16 }}>
        <div>
          <h1 className="sl-page-title">Weekly Review</h1>
          <p className="sl-page-sub">Week of Apr 19 – Apr 25 · 4 active campaigns</p>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button variant="secondary" size="sm">Export CSV</Button>
          <Button variant="primary" size="sm"><Icon.Plus/> New campaign</Button>
        </div>
      </div>
      <HeroStatsBar/>
      <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 16, marginBottom: 24 }}>
        <div className="sl-card">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 14 }}>
            <h3 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "var(--color-heading)" }}>Recent Sales</h3>
            <StatusPill tone="success">+$466 today</StatusPill>
          </div>
          <LedgerTable/>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div className="sl-card premium">
            <div style={{ fontSize: 10, color: "var(--brand-400)", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 600, marginBottom: 8 }}>Insight · AI</div>
            <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-heading)", marginBottom: 8 }}>PSA 10 Scarlet & Violet climbing</div>
            <p style={{ fontSize: 12, color: "var(--text-muted)", lineHeight: 1.6, margin: 0 }}>Prismatic Evolutions PSA 10s rose 8.4% this week. Three inventory items are candidates for re-list at +12%.</p>
            <div style={{ marginTop: 14 }}><RecommendationBadge tier="STRONG BUY"/></div>
          </div>
          <div className="sl-card">
            <h3 style={{ margin: "0 0 12px", fontSize: 14, fontWeight: 600, color: "var(--color-heading)" }}>Campaign P&L</h3>
            {[
              { n: "Prismatic Direct Buy", roi: "+22.1%", pos: true },
              { n: "151 Set Recovery", roi: "+9.8%", pos: true },
              { n: "Paldea Evolved Batch", roi: "−3.4%", pos: false },
              { n: "Obsidian Flames · test", roi: "+14.2%", pos: true },
            ].map((c, i) => (
              <div key={i} style={{ display: "flex", justifyContent: "space-between", padding: "8px 0", borderBottom: i < 3 ? "1px solid rgba(255,255,255,0.04)" : "0", fontSize: 13 }}>
                <span style={{ color: "var(--text)" }}>{c.n}</span>
                <span className={c.pos ? "sl-pos" : "sl-neg"} style={{ fontVariantNumeric: "tabular-nums", fontWeight: 600 }}>{c.roi}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function CampaignsPage() {
  const campaigns = [
    { name: "Prismatic Direct Buy", status: "success", statusLabel: "Active", spend: "$12,840 / $15k", fill: "87%", cards: 142, roi: "+22.1%", pos: true },
    { name: "151 Set Recovery", status: "success", statusLabel: "Active", spend: "$8,200 / $10k", fill: "92%", cards: 218, roi: "+9.8%", pos: true },
    { name: "Paldea Evolved Batch", status: "warning", statusLabel: "Slow", spend: "$18,900 / $20k", fill: "64%", cards: 310, roi: "−3.4%", pos: false },
    { name: "Obsidian Flames · test", status: "info", statusLabel: "Pilot", spend: "$2,400 / $5k", fill: "48%", cards: 36, roi: "+14.2%", pos: true },
    { name: "Temporal Forces", status: "neutral", statusLabel: "Paused", spend: "$0 / $10k", fill: "—", cards: 0, roi: "—", pos: true },
  ];
  return (
    <div data-screen-label="03 Campaigns">
      <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <h1 className="sl-page-title">Campaigns</h1>
          <p className="sl-page-sub">5 total · 3 active · $42,340 deployed this month</p>
        </div>
        <Button variant="primary"><Icon.Plus/> New campaign</Button>
      </div>
      <table className="sl-table">
        <thead>
          <tr>
            <th>Campaign</th>
            <th>Status</th>
            <th className="num">Spend</th>
            <th className="num">Fill rate</th>
            <th className="num">Cards</th>
            <th className="num">ROI</th>
          </tr>
        </thead>
        <tbody>
          {campaigns.map((c, i) => (
            <tr key={i}>
              <td style={{ fontWeight: 600 }}>{c.name}</td>
              <td><StatusPill tone={c.status}>{c.statusLabel.toUpperCase()}</StatusPill></td>
              <td className="num">{c.spend}</td>
              <td className="num">{c.fill}</td>
              <td className="num">{c.cards}</td>
              <td className={`num ${c.pos ? "sl-pos" : "sl-neg"}`} style={{ fontWeight: 600 }}>{c.roi}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function InventoryPage() {
  const rows = [
    { name: "Charizard ex · Obsidian Flames", set: "Scarlet & Violet", grader: "PSA", grade: "10", deployed: "$420.00", market: "$510 ↗", direction: "rising", daysHeld: 14, rec: "STRONG BUY" },
    { name: "Iono Full Art · Paldea Evolved", set: "Scarlet & Violet", grader: "PSA", grade: "10", deployed: "$280.00", market: "$318", direction: "stable", daysHeld: 32, rec: "BUY" },
    { name: "Lugia V Alt · Silver Tempest", set: "Sword & Shield", grader: "BGS", grade: "10", deployed: "$880.00", market: "$740 ↘", direction: "falling", daysHeld: 71, rec: "BUY WITH CAUTION" },
    { name: "Mew ex Gold · 151", set: "Scarlet & Violet", grader: "PSA", grade: "10", deployed: "$640.00", market: "$820 ↗", direction: "rising", daysHeld: 8, rec: "MUST BUY" },
    { name: "Umbreon VMAX Alt · Evolving Skies", set: "Sword & Shield", grader: "PSA", grade: "9", deployed: "$1,420.00", market: "$1,180 ↘", direction: "falling", daysHeld: 94, rec: "AVOID" },
    { name: "Giratina V Alt · Lost Origin", set: "Sword & Shield", grader: "CGC", grade: "9.5", deployed: "$360.00", market: "$392", direction: "stable", daysHeld: 22, rec: "WATCH" },
  ];
  return (
    <div data-screen-label="04 Inventory">
      <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", marginBottom: 20 }}>
        <div>
          <h1 className="sl-page-title">Inventory</h1>
          <p className="sl-page-sub">842 cards unsold · $48,920 deployed · avg age 31 days</p>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <Button variant="secondary" size="sm">Filters</Button>
          <Button variant="secondary" size="sm">Export</Button>
        </div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1.5fr 100px 120px 120px 140px 110px", gap: 12, padding: "0 18px 8px", fontSize: 10, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>
        <div>Card</div>
        <div>Grade</div>
        <div>Deployed</div>
        <div>Market</div>
        <div>Age</div>
        <div>Rec</div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {rows.map((r, i) => <InventoryRow key={i} {...r}/>)}
      </div>
    </div>
  );
}

function App() {
  const [signedIn, setSignedIn] = useState(false);
  const [route, setRoute] = useState("dashboard");
  if (!signedIn) return <LoginPage onSignIn={() => setSignedIn(true)}/>;
  return (
    <>
      <Header route={route} setRoute={setRoute} onSignOut={() => setSignedIn(false)}/>
      <main className="sl-main">
        {route === "dashboard" && <DashboardPage/>}
        {route === "campaigns" && <CampaignsPage/>}
        {route === "inventory" && <InventoryPage/>}
        {(route === "reprice" || route === "insights" || route === "invoices") && (
          <EmptyState
            icon={route === "insights" ? "🔮" : route === "invoices" ? "🧾" : "💱"}
            title={`${route[0].toUpperCase() + route.slice(1)} coming soon`}
            description="This surface is stubbed in the UI kit. See the live app for the full implementation."
            steps={["Return to Dashboard", "Explore Campaigns", "Browse Inventory"]}
          />
        )}
      </main>
    </>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App/>);
