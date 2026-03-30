const CL_EMAIL = process.env.CL_EMAIL || 'thomasgamble2@gmail.com';
const CL_PASSWORD = process.env.CL_PASSWORD;
const FIREBASE_API_KEY = 'AIzaSyBqbxgaaGlpeb1F6HRvEW319OcuCsbkAHM';
const COLLECTION_ID = '4ROuX7h0KsZOcenGKsQX';

if (!CL_PASSWORD) {
  console.error('Set CL_PASSWORD env var');
  process.exit(1);
}

// Step 1: Get Firebase token via REST API
const loginResp = await fetch(
  `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${FIREBASE_API_KEY}`,
  {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: CL_EMAIL, password: CL_PASSWORD, returnSecureToken: true }),
  }
);
const loginData = await loginResp.json();
if (!loginData.idToken) {
  console.error('Firebase login failed:', loginData);
  process.exit(1);
}
console.log('Firebase login OK');

const token = loginData.idToken;

// Step 2: Fetch first page of collection cards
const collResp = await fetch(
  `https://search-zzvl7ri3bq-uc.a.run.app/search?index=collectioncards&query=&page=0&limit=5&filters=collectionId:${COLLECTION_ID}|hasQuantityAvailable:true&sort=player&direction=asc`,
  { headers: { Authorization: `Bearer ${token}` } }
);
const collData = await collResp.json();

console.log('\n=== COLLECTION CARD SAMPLE (first card, all fields) ===');
if (collData.hits && collData.hits.length > 0) {
  console.log(JSON.stringify(collData.hits[0], null, 2));

  // Check all keys across first 5 cards
  const allKeys = new Set();
  for (const card of collData.hits) {
    Object.keys(card).forEach(k => allKeys.add(k));
  }
  console.log('\n=== ALL KEYS ACROSS FIRST 5 CARDS ===');
  console.log([...allKeys].sort().join('\n'));

  // Specifically look for gemRateId or similar
  const gemKeys = [...allKeys].filter(k => k.toLowerCase().includes('gem') || k.toLowerCase().includes('rate') || k.toLowerCase().includes('id'));
  console.log('\n=== KEYS CONTAINING gem/rate/id ===');
  console.log(gemKeys.join('\n'));
} else {
  console.log('No hits:', collData);
}

// Step 3: Try different search strategies on "cards" index
const first = collData.hits[2]; // Use the Venusaur card we know has sales
console.log(`\nTarget: ${first.label}`);

// Strategy A: No filters at all, just query
const strategies = [
  { name: 'query only (player)', query: first.player, filters: '' },
  { name: 'query (player+number)', query: `${first.player} ${first.number}`, filters: '' },
  { name: 'query (label)', query: first.label, filters: '' },
  { name: 'query (player) + set filter', query: first.player, filters: `set:${first.set}` },
  { name: 'query (player) + category filter', query: first.player, filters: `category:${first.category}` },
  { name: 'query (venusaur 198)', query: 'venusaur 198', filters: '' },
  { name: 'query (venusaur ex 198 151)', query: 'venusaur ex 198 151', filters: '' },
];

for (const s of strategies) {
  const params = new URLSearchParams({
    index: 'cards',
    query: s.query,
    page: '0',
    limit: '3',
  });
  if (s.filters) params.set('filters', s.filters);

  const resp = await fetch(
    `https://search-zzvl7ri3bq-uc.a.run.app/search?${params}`,
    { headers: { Authorization: `Bearer ${token}` } }
  );
  const data = await resp.json();
  console.log(`\n--- ${s.name} ---`);
  console.log(`  Hits: ${data.totalHits}`);
  for (const h of (data.hits || []).slice(0, 2)) {
    console.log(`  ${h.label}`);
    console.log(`    id=${h.id} gemRateId=${h.gemRateId} condition=${h.condition}`);
  }
}

// Step 4: Check if there's a Firestore path we can use
// The collection card has a collectionCardId — maybe Firestore has the link
console.log('\n=== CHECKING FIRESTORE FOR CARD LINK ===');
const ccId = collData.hits[0].collectionCardId;
const collId = collData.hits[0].collectionId;
console.log(`collectionCardId: ${ccId}, collectionId: ${collId}`);

// Try fetching the Firestore document directly
try {
  const fsResp = await fetch(
    `https://firestore.googleapis.com/v1/projects/cardladder-71d53/databases/(default)/documents/collections/${collId}/cards/${ccId}`,
    { headers: { Authorization: `Bearer ${token}` } }
  );
  const fsData = await fsResp.json();
  console.log('Firestore doc keys:', Object.keys(fsData.fields || fsData));
  console.log(JSON.stringify(fsData, null, 2).slice(0, 2000));
} catch (e) {
  console.log('Firestore fetch failed:', e.message);
}
