// SPDX-License-Identifier: AGPL-3.0-only

// Signs a document with RsaSignature2017 as Mastodon and Misskey do, with jsonld.js (8.3.3) and
// the bundled contexts, and prints the canonical N-Quads and the signature: the
// cross-implementation vector of federationtest.CanonicalizerContract (create-note.jsonld-js.json). Not run by the
// tests. In a directory with jsonld installed:
//
//	node sign.mjs ../../../../../internal/adapter/jsonld/contexts key.pem create-note.json \
//	  'https://my.example/ap/actors/01a148a1-365b-79f7-929a-781395cc457f#main-key' 2026-10-17T06:53:04Z
import fs from 'node:fs';
import crypto from 'node:crypto';
import path from 'node:path';
import jsonld from 'jsonld';

const [ctxdir, keyFile, docFile, creator, created] = process.argv.slice(2);
const contexts = {
  'https://www.w3.org/ns/activitystreams': 'as.jsonld',
  'https://w3id.org/security/v1': 'security-v1.jsonld',
  'https://w3id.org/identity/v1': 'identity-v1.jsonld',
};
const documentLoader = async (url) => {
  const f = contexts[url];
  if (!f) throw new Error('not bundled: ' + url);
  return { contextUrl: null, documentUrl: url, document: JSON.parse(fs.readFileSync(path.join(ctxdir, f), 'utf8')) };
};
const canon = (d) => jsonld.canonize(d, { algorithm: 'URDNA2015', format: 'application/n-quads', documentLoader, safe: false });
const sha256 = (s) => crypto.createHash('sha256').update(s).digest('hex');

const doc = JSON.parse(fs.readFileSync(docFile, 'utf8'));
const options = { type: 'RsaSignature2017', creator, created };
const o = { ...options, '@context': 'https://w3id.org/identity/v1' };
delete o.type; delete o.id; delete o.signatureValue;
const on = await canon(o);
const d = { ...doc }; delete d.signature;
const dn = await canon(d);
const verifyData = sha256(on) + sha256(dn);
const sig = crypto.createSign('sha256').update(verifyData).sign(fs.readFileSync(keyFile, 'utf8'), 'base64');
console.log(JSON.stringify({ options_nquads: on, document_nquads: dn, signatureValue: sig }, null, 2));
