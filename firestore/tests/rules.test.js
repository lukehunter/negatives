// Security rules tests for firestore/firestore.rules (see docs/testing.md).
// Every `allow` gets at least one allowed and one denied case, plus checks
// that other users, signed-out clients and unknown collections are denied.
//
// Run from the repo root (starts the emulator, runs, stops it):
//   firebase emulators:exec --only firestore --project demo-negatives "npm test --prefix firestore/tests"
//
// The demo- project ID keeps the emulator offline: nothing here can reach
// the real project.

import { readFileSync } from 'node:fs';
import { after, before, beforeEach, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  assertFails,
  assertSucceeds,
  initializeTestEnvironment,
} from '@firebase/rules-unit-testing';
import { deleteDoc, doc, getDoc, setDoc, setLogLevel, Timestamp, updateDoc } from 'firebase/firestore';

// RULES_FILE lets a deliberately broken copy be tested, to check that the
// tests notice.
const rulesPath = process.env.RULES_FILE ?? fileURLToPath(new URL('../firestore.rules', import.meta.url));
const rules = readFileSync(rulesPath, 'utf8');

// Denied writes are the point of most tests; don't log each one.
setLogLevel('silent');

// The owner is whoever the rules name, so the tests follow the file.
const ownerMatch = rules.match(/request\.auth\.uid == '([^']+)'/);
if (!ownerMatch) throw new Error('no owner UID found in firestore.rules');
const OWNER = ownerMatch[1];
const STRANGER = 'someone-else';

const MD5 = '0123456789abcdef0123456789abcdef';
const PATH_KEY = '0123456789abcdef';
const now = () => Timestamp.now();

// The JS SDK stores only safe integers as integers, so test values stay
// below 2^53; the Swift tools write real nanosecond times as Int64.
const MTIME_NS = 1761436800123456;

function cardCopy(extra = {}) {
  return {
    schema: 1,
    loc: 'card',
    path: 'DCIM/100MSDCF/DSC00001.ARW',
    size: 24_000_000,
    mtimeNs: MTIME_NS,
    seenAt: now(),
    card: { cardKey: '1a2b3c4d-0000-4000-8000-000000000001', sessionId: 'session-1' },
    ...extra,
  };
}

function ssdCopy(extra = {}, ssdExtra = {}) {
  return {
    schema: 1,
    loc: 'ssd',
    path: '2026-10-18 Coast/.originals/ILCE-7M4-01/DCIM/100MSDCF/DSC00001.ARW',
    size: 24_000_000,
    mtimeNs: MTIME_NS,
    seenAt: now(),
    ssd: {
      originalsPath: '2026-10-18 Coast/.originals/ILCE-7M4-01/DCIM/100MSDCF/DSC00001.ARW',
      workingPath: '2026-10-18 Coast/ILCE-7M4-01/DCIM/100MSDCF/DSC00001.ARW',
      workingSize: 24_000_000,
      workingMtimeNs: MTIME_NS,
      copiedAt: now(),
      pushedAt: null,
      modified: false,
      ...ssdExtra,
    },
    ...extra,
  };
}

function cardSession(extra = {}) {
  return {
    schema: 1,
    volumeUuid: '1a2b3c4d-0000-4000-8000-000000000001',
    shoot: '2026-10-18 Coast',
    cardFolder: 'ILCE-7M4-01',
    cameraModel: 'ILCE-7M4',
    cardLabel: 'S01',
    capacityBytes: 128_000_000_000,
    firstIngestAt: now(),
    lastSeenAt: now(),
    samples: [{ path: 'DCIM/100MSDCF/DSC00001.ARW', size: 24_000_000 }],
    stats: { readErrors: 0, hashMismatches: 0 },
    ...extra,
  };
}

let env;

before(async () => {
  env = await initializeTestEnvironment({
    projectId: 'demo-negatives',
    firestore: { rules },
  });
});

after(async () => {
  await env?.cleanup();
});

beforeEach(async () => {
  await env.clearFirestore();
});

const owner = () => env.authenticatedContext(OWNER).firestore();
const stranger = () => env.authenticatedContext(STRANGER).firestore();
const signedOut = () => env.unauthenticatedContext().firestore();

// Writes as the Windows tools would: their service account bypasses rules.
async function seed(path, data) {
  await env.withSecurityRulesDisabled(async (ctx) => {
    await setDoc(doc(ctx.firestore(), path), data);
  });
}

describe('files/{md5}', () => {
  const path = `files/${MD5}`;

  it('owner reads; others and signed-out clients do not', async () => {
    await seed(path, { schema: 1, size: 10 });
    await assertSucceeds(getDoc(doc(owner(), path)));
    await assertFails(getDoc(doc(stranger(), path)));
    await assertFails(getDoc(doc(signedOut(), path)));
  });

  it('owner creates a summary with only schema and size', async () => {
    await assertSucceeds(setDoc(doc(owner(), path), { schema: 1, size: 10 }, { merge: true }));
  });

  it('create is denied for archive fields, bad sizes, other schemas and bad IDs', async () => {
    await assertFails(setDoc(doc(owner(), path), { schema: 1, size: 10, primary: { copies: 1 } }));
    await assertFails(setDoc(doc(owner(), path), { schema: 1, size: 0 }));
    await assertFails(setDoc(doc(owner(), path), { schema: 1, size: 1.5 }));
    await assertFails(setDoc(doc(owner(), path), { schema: 2, size: 10 }));
    await assertFails(setDoc(doc(owner(), path), { size: 10 }));
    await assertFails(setDoc(doc(owner(), `files/${MD5.toUpperCase()}`), { schema: 1, size: 10 }));
    await assertFails(setDoc(doc(owner(), 'files/not-an-md5'), { schema: 1, size: 10 }));
    await assertFails(setDoc(doc(stranger(), path), { schema: 1, size: 10 }));
  });

  it('owner may re-merge schema and size over a summary Windows maintains', async () => {
    await seed(path, { schema: 1, size: 10, primary: { copies: 1 }, backup: { copies: 1 }, shoots: 1, common: false });
    await assertSucceeds(setDoc(doc(owner(), path), { schema: 1, size: 10 }, { merge: true }));
  });

  it('update is denied for a size change or any archive field', async () => {
    await seed(path, { schema: 1, size: 10, primary: { copies: 1 } });
    await assertFails(setDoc(doc(owner(), path), { schema: 1, size: 11 }, { merge: true }));
    await assertFails(updateDoc(doc(owner(), path), { 'primary.copies': 2 }));
    await assertFails(updateDoc(doc(owner(), path), { common: true }));
    await assertFails(updateDoc(doc(stranger(), path), { size: 10 }));
  });

  it('delete is always denied', async () => {
    await seed(path, { schema: 1, size: 10 });
    await assertFails(deleteDoc(doc(owner(), path)));
  });
});

describe('files/{md5}/paths/{pathKey}', () => {
  const path = `files/${MD5}/paths/${PATH_KEY}`;

  it('owner reads; others do not', async () => {
    await seed(path, cardCopy());
    await assertSucceeds(getDoc(doc(owner(), path)));
    await assertFails(getDoc(doc(stranger(), path)));
    await assertFails(getDoc(doc(signedOut(), path)));
  });

  it('owner creates card and SSD copies', async () => {
    await assertSucceeds(setDoc(doc(owner(), path), cardCopy(), { merge: true }));
    await assertSucceeds(setDoc(doc(owner(), `files/${MD5}/paths/fedcba9876543210`), ssdCopy(), { merge: true }));
  });

  it('a legacy SSD copy has no originals path', async () => {
    const legacy = ssdCopy({ path: '2019-05-04 Beach/DSC00001.JPG' }, {
      originalsPath: null,
      workingPath: '2019-05-04 Beach/DSC00001.JPG',
    });
    await assertSucceeds(setDoc(doc(owner(), path), legacy));
  });

  it('create is denied for archive locations, bad keys, bad paths and extra fields', async () => {
    const denied = [
      cardCopy({ loc: 'primary' }),
      cardCopy({ loc: 'backup' }),
      // Shaped like an SSD copy, so only the loc check can deny them.
      ssdCopy({ loc: 'primary' }),
      ssdCopy({ loc: 'backup' }),
      cardCopy({ schema: 2 }),
      cardCopy({ size: 0 }),
      cardCopy({ mtimeNs: 1.5 }),
      cardCopy({ seenAt: '2026-10-25' }),
      cardCopy({ path: '/DCIM/DSC00001.ARW' }),
      cardCopy({ path: 'DCIM/DSC00001.ARW/' }),
      cardCopy({ path: 'DCIM/../DSC00001.ARW' }),
      cardCopy({ path: 'DCIM/./DSC00001.ARW' }),
      cardCopy({ path: 'DCIM//DSC00001.ARW' }),
      cardCopy({ path: 'DCIM/DSC:00001.ARW' }),
      cardCopy({ path: '' }),
      cardCopy({ path: 'a'.repeat(1025) }),
      cardCopy({ verifiedAt: now() }),
      cardCopy({ card: { cardKey: 'x' } }),
      cardCopy({ card: { cardKey: 'x', sessionId: 's', extra: 1 } }),
      ssdCopy({}, { workingPath: '../escape.ARW' }),
      ssdCopy({}, { surprise: true }),
    ];
    for (const data of denied) {
      await assertFails(setDoc(doc(owner(), path), data));
    }
    await assertFails(setDoc(doc(owner(), `files/${MD5}/paths/NOT-A-KEY`), cardCopy()));
    await assertFails(setDoc(doc(owner(), `files/${MD5}/paths/0123456789abcdef0`), cardCopy()));
    await assertFails(setDoc(doc(stranger(), path), cardCopy()));
  });

  it('owner updates its own copy, keeping loc and path', async () => {
    await seed(path, cardCopy());
    await assertSucceeds(setDoc(doc(owner(), path), cardCopy({ seenAt: now() }), { merge: true }));
  });

  it('update is denied for a new path, a new loc, or a Windows copy', async () => {
    await seed(path, cardCopy());
    await assertFails(setDoc(doc(owner(), path), cardCopy({ path: 'DCIM/100MSDCF/DSC00002.ARW' })));
    await assertFails(setDoc(doc(owner(), path), ssdCopy({ path: cardCopy().path })));
    const archive = `files/${MD5}/paths/1111111111111111`;
    await seed(archive, { schema: 1, loc: 'primary', path: '2026-10-18 Coast/Originals/DSC00001.ARW', size: 10, mtimeNs: MTIME_NS, seenAt: now(), verifiedAt: now() });
    await assertFails(setDoc(doc(owner(), archive), cardCopy({ path: '2026-10-18 Coast/Originals/DSC00001.ARW' })));
  });

  it('delete is allowed only for SSD copies', async () => {
    await seed(path, ssdCopy());
    await assertSucceeds(deleteDoc(doc(owner(), path)));
    await seed(path, cardCopy());
    await assertFails(deleteDoc(doc(owner(), path)));
    await seed(path, { schema: 1, loc: 'backup', path: 'x/y', size: 10, mtimeNs: MTIME_NS, seenAt: now() });
    await assertFails(deleteDoc(doc(owner(), path)));
    await seed(path, ssdCopy());
    await assertFails(deleteDoc(doc(stranger(), path)));
  });
});

describe('cardSessions/{sessionId}', () => {
  const path = 'cardSessions/session-1';

  it('owner reads; others do not', async () => {
    await seed(path, cardSession());
    await assertSucceeds(getDoc(doc(owner(), path)));
    await assertFails(getDoc(doc(stranger(), path)));
  });

  it('owner creates and updates a session', async () => {
    await assertSucceeds(setDoc(doc(owner(), path), cardSession()));
    await assertSucceeds(setDoc(doc(owner(), path), { lastSeenAt: now() }, { merge: true }));
  });

  it('is denied for extra fields, bad paths, too many samples or another schema', async () => {
    await assertFails(setDoc(doc(owner(), path), cardSession({ surprise: 1 })));
    await assertFails(setDoc(doc(owner(), path), cardSession({ shoot: '../Coast' })));
    await assertFails(setDoc(doc(owner(), path), cardSession({ cardFolder: 'a/' })));
    await assertFails(setDoc(doc(owner(), path), cardSession({ samples: Array(6).fill({ path: 'a', size: 1 }) })));
    await assertFails(setDoc(doc(owner(), path), cardSession({ schema: 2 })));
    await assertFails(setDoc(doc(owner(), path), cardSession({ volumeUuid: 42 })));
    await assertFails(setDoc(doc(stranger(), path), cardSession()));
  });

  it('delete is always denied', async () => {
    await seed(path, cardSession());
    await assertFails(deleteDoc(doc(owner(), path)));
  });
});

describe('status/{name}', () => {
  it('owner reads any status; others do not', async () => {
    await seed('status/archive-hasher', { schema: 1, heartbeatAt: now() });
    await assertSucceeds(getDoc(doc(owner(), 'status/archive-hasher')));
    await assertFails(getDoc(doc(stranger(), 'status/archive-hasher')));
  });

  it('owner may request a sync', async () => {
    await assertSucceeds(setDoc(doc(owner(), 'status/sync-request'), { schema: 1, requestedAt: now() }));
  });

  it('owner may not write any other status, or extra fields', async () => {
    await assertFails(setDoc(doc(owner(), 'status/archive-hasher'), { schema: 1, requestedAt: now() }));
    await assertFails(setDoc(doc(owner(), 'status/sync-request'), { schema: 1, requestedAt: now(), surprise: 1 }));
    await assertFails(setDoc(doc(owner(), 'status/sync-request'), { schema: 1, requestedAt: 'now' }));
    await assertFails(setDoc(doc(stranger(), 'status/sync-request'), { schema: 1, requestedAt: now() }));
  });

  it('delete is always denied', async () => {
    await seed('status/sync-request', { schema: 1, requestedAt: now() });
    await assertFails(deleteDoc(doc(owner(), 'status/sync-request')));
  });
});

describe('config/{name}', () => {
  const settings = 'config/settings';

  it('owner reads; others do not', async () => {
    await seed(settings, { schema: 1 });
    await assertSucceeds(getDoc(doc(owner(), settings)));
    await assertFails(getDoc(doc(stranger(), settings)));
  });

  it('owner writes valid shared settings', async () => {
    await assertSucceeds(setDoc(doc(owner(), settings), {
      schema: 1,
      derivedSettleMinutes: 60,
      watcherQuietMinutes: 10,
      commonShootThreshold: 10,
      exclude: ['*.lrprev'],
      cameraNames: { 'ILCE-7RM5': 'a7rv' },
      updatedAt: now(),
      updatedBy: 'mac',
    }));
    await assertSucceeds(setDoc(doc(owner(), settings), { schema: 1 }));
  });

  it('is denied for other documents, unknown keys and out-of-range values', async () => {
    await assertFails(setDoc(doc(owner(), 'config/other'), { schema: 1 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, surprise: 1 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 2 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, derivedSettleMinutes: 0 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, watcherQuietMinutes: 2.5 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, commonShootThreshold: 1 }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, exclude: Array(201).fill('x') }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, exclude: 'x' }));
    await assertFails(setDoc(doc(owner(), settings), { schema: 1, cameraNames: ['a7rv'] }));
    await assertFails(setDoc(doc(stranger(), settings), { schema: 1 }));
  });

  it('delete is always denied', async () => {
    await seed(settings, { schema: 1 });
    await assertFails(deleteDoc(doc(owner(), settings)));
  });
});

describe('conflicts/{id}', () => {
  const path = 'conflicts/c1';
  const conflict = { schema: 1, kind: 'content-changed', disk: 'primary', path: 'x/y', md5: MD5, detectedAt: Timestamp.now(), details: '' };

  it('owner reads; others do not', async () => {
    await seed(path, conflict);
    await assertSucceeds(getDoc(doc(owner(), path)));
    await assertFails(getDoc(doc(stranger(), path)));
  });

  it('owner marks a conflict resolved', async () => {
    await seed(path, conflict);
    await assertSucceeds(updateDoc(doc(owner(), path), { resolvedAt: now(), resolution: 'kept the original' }));
  });

  it('owner may change nothing else, and may not create or delete', async () => {
    await seed(path, conflict);
    await assertFails(updateDoc(doc(owner(), path), { resolvedAt: now(), resolution: 'x', kind: 'missing' }));
    await assertFails(updateDoc(doc(owner(), path), { resolvedAt: 'today', resolution: 'x' }));
    await assertFails(updateDoc(doc(stranger(), path), { resolvedAt: now(), resolution: 'x' }));
    await assertFails(setDoc(doc(owner(), 'conflicts/c2'), conflict));
    await assertFails(deleteDoc(doc(owner(), path)));
  });
});

describe('everything else', () => {
  it('unknown collections are denied, even to the owner', async () => {
    await assertFails(setDoc(doc(owner(), 'spikes/x'), { schema: 1 }));
    await assertFails(getDoc(doc(owner(), 'spikes/x')));
    await assertFails(setDoc(doc(owner(), 'files/' + MD5 + '/other/x'), { schema: 1 }));
  });

  it('signed-out clients can do nothing', async () => {
    await assertFails(setDoc(doc(signedOut(), `files/${MD5}`), { schema: 1, size: 10 }));
    await assertFails(setDoc(doc(signedOut(), 'status/sync-request'), { schema: 1, requestedAt: now() }));
    await assertFails(getDoc(doc(signedOut(), 'config/settings')));
  });
});
