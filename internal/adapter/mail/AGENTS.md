# Outgoing mail: message format, SMTP, Amazon SES

- Changing a header, its folding, or an address of a message: [RFC 5322](https://www.rfc-editor.org/rfc/rfc5322), search for: `Message-ID`, Header Fields, Long Header Fields, Address Specification
- Encoding a non-ASCII name or subject: [RFC 2047](https://www.rfc-editor.org/rfc/rfc2047), search for: `encoded-word`, 75 characters
- Changing the parts of a message (`multipart/alternative`, boundaries): [RFC 2046](https://www.rfc-editor.org/rfc/rfc2046), search for: `multipart/alternative`, boundary
- Marking a message as automatic: [RFC 3834](https://www.rfc-editor.org/rfc/rfc3834), search for: `Auto-Submitted`, `auto-generated`
- Changing the unsubscribe headers: [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058), search for: `List-Unsubscribe-Post`, `List-Unsubscribe=One-Click`
- Changing the SMTP dialogue or the reading of a reply code: [RFC 5321](https://www.rfc-editor.org/rfc/rfc5321), search for: `RCPT`, Reply Codes, 4yz, 5yz
- Requiring or negotiating TLS on SMTP: [RFC 3207](https://www.rfc-editor.org/rfc/rfc3207), search for: `STARTTLS`, man-in-the-middle
- Classifying a failure by its enhanced status code: [RFC 3463](https://www.rfc-editor.org/rfc/rfc3463), search for: Addressing Status, `X.1.1`, Permanent Failure
- Calling SES: [Amazon SES API v2 Reference](https://docs.aws.amazon.com/ses/latest/APIReference-V2/llms.txt), search for: `SendEmail`, `Raw`, `ConfigurationSetName`, `MessageRejected`
- Relying on what SES does to a message (its own Message-ID and Date, no SMTPUTF8, sending limits): [Amazon SES Developer Guide](https://docs.aws.amazon.com/ses/latest/dg/llms.txt), search for: `Message-ID`, sending quotas, raw email
