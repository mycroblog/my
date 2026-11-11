# Key-encryption keys: AWS KMS, Cloud KMS, local AES-256-GCM

- Calling AWS KMS or changing the encryption context: [AWS KMS Developer Guide](https://docs.aws.amazon.com/kms/latest/developerguide/llms.txt), search for: Encryption context, `Encrypt`, `Decrypt`, `DescribeKey`, key ARN
- Relying on how AWS KMS rotates a key or resolves an alias: [AWS KMS Developer Guide](https://docs.aws.amazon.com/kms/latest/developerguide/llms.txt), search for: Rotate AWS KMS keys, alias, key material
- Calling Cloud KMS or changing the additional authenticated data: [Cloud KMS documentation](https://docs.cloud.google.com/kms/docs), search for: Additional authenticated data, `CryptoKey`, `plaintext_crc32c`, Key rotation
- Changing the nonce or the tag of the local AES-256-GCM wrapping: [NIST SP 800-38D](https://csrc.nist.gov/pubs/sp/800/38/d/final), search for: Galois/Counter Mode, GMAC
