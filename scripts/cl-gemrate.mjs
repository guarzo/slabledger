const CL_EMAIL = process.env.CL_EMAIL || 'thomasgamble2@gmail.com';
const CL_PASSWORD = process.env.CL_PASSWORD;
const FIREBASE_API_KEY = 'AIzaSyBqbxgaaGlpeb1F6HRvEW319OcuCsbkAHM';
const COLLECTION_ID = '4ROuX7h0KsZOcenGKsQX';
const UID = 'xzl4x4fwsMXvmpx0IlNq6y8ZMeS2';

if (!CL_PASSWORD) { console.error('Set CL_PASSWORD env var'); process.exit(1); }

// Firebase login
const loginResp = await fetch(
  `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${FIREBASE_API_KEY}`,
  {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: CL_EMAIL, password: CL_PASSWORD, returnSecureToken: true }),
  }
);
const { idToken: token } = await loginResp.json();
console.log('Firebase login OK');

// Try the correct Firestore path
const cardId = 'V5ZwnMh9iLOASoMyCZc8';
const docPath = `users/${UID}/collections/${COLLECTION_ID}/collection_cards/${cardId}`;
console.log(`\n=== Firestore REST: ${docPath} ===`);

const fsResp = await fetch(
  `https://firestore.googleapis.com/v1/projects/cardladder-71d53/databases/(default)/documents/${docPath}`,
  { headers: { Authorization: `Bearer ${token}` } }
);
const fsData = await fsResp.json();

if (fsData.fields) {
  console.log('SUCCESS! Key fields:');
  console.log(`  gemRateId: ${fsData.fields.gemRateId?.stringValue}`);
  console.log(`  gemRateCondition: ${fsData.fields.gemRateCondition?.stringValue}`);
  console.log(`  slabSerial: ${fsData.fields.slabSerial?.stringValue}`);
  console.log(`  player: ${fsData.fields.player?.stringValue}`);
  console.log(`  set: ${fsData.fields.set?.stringValue}`);

  // Now try batch: list ALL collection cards to get all gemRateIds
  console.log('\n=== Listing all collection cards from Firestore ===');
  const listPath = `users/${UID}/collections/${COLLECTION_ID}/collection_cards`;
  let allCards = [];
  let pageToken = '';

  for (let i = 0; i < 10; i++) { // max 10 pages
    const listUrl = new URL(`https://firestore.googleapis.com/v1/projects/cardladder-71d53/databases/(default)/documents/${listPath}`);
    listUrl.searchParams.set('pageSize', '100');
    if (pageToken) listUrl.searchParams.set('pageToken', pageToken);

    const listResp = await fetch(listUrl, { headers: { Authorization: `Bearer ${token}` } });
    const listData = await listResp.json();

    if (listData.documents) {
      allCards.push(...listData.documents);
    }

    if (!listData.nextPageToken) break;
    pageToken = listData.nextPageToken;
  }

  console.log(`Total documents: ${allCards.length}`);

  let withGemRate = 0;
  let withoutGemRate = 0;

  for (const doc of allCards) {
    const f = doc.fields;
    const gemId = f?.gemRateId?.stringValue;
    const gemCond = f?.gemRateCondition?.stringValue;
    const serial = f?.slabSerial?.stringValue;
    const label = f?.label?.stringValue;

    if (gemId) {
      withGemRate++;
      if (withGemRate <= 5) {
        console.log(`  [HAS] ${label} | gemRateId=${gemId} | cond=${gemCond} | serial=${serial}`);
      }
    } else {
      withoutGemRate++;
      if (withoutGemRate <= 3) {
        console.log(`  [MISSING] ${label} | serial=${serial}`);
      }
    }
  }

  console.log(`\nSummary: ${withGemRate} with gemRateId, ${withoutGemRate} without`);
} else {
  console.log('FAILED:', JSON.stringify(fsData, null, 2).slice(0, 500));
}
