(function exposeWebAuthn(global) {
  function decodeBase64URL(value) {
    const padding = '='.repeat((4 - (value.length % 4)) % 4);
    const binary = atob(value.replace(/-/g, '+').replace(/_/g, '/') + padding);
    return Uint8Array.from(binary, (character) => character.charCodeAt(0));
  }

  function encodeBase64URL(value) {
    if (!value) return '';
    const bytes = new Uint8Array(value);
    let binary = '';
    bytes.forEach((byte) => { binary += String.fromCharCode(byte); });
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  function registrationOptions(publicKey) {
    return {
      ...publicKey,
      challenge: decodeBase64URL(publicKey.challenge),
      user: { ...publicKey.user, id: decodeBase64URL(publicKey.user.id) },
      excludeCredentials: (publicKey.excludeCredentials || []).map((item) => ({
        ...item, id: decodeBase64URL(item.id),
      })),
    };
  }

  function authenticationOptions(publicKey) {
    return {
      ...publicKey,
      challenge: decodeBase64URL(publicKey.challenge),
      allowCredentials: (publicKey.allowCredentials || []).map((item) => ({
        ...item, id: decodeBase64URL(item.id),
      })),
    };
  }

  async function create(publicKey) {
    if (!global.PublicKeyCredential || !navigator.credentials) {
      throw new Error('Dieser Browser unterstützt keine Passkeys.');
    }
    const credential = await navigator.credentials.create({ publicKey: registrationOptions(publicKey) });
    if (!credential) throw new Error('Die Passkey-Erstellung wurde abgebrochen.');
    return {
      id: credential.id,
      rawId: encodeBase64URL(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      response: {
        clientDataJSON: encodeBase64URL(credential.response.clientDataJSON),
        attestationObject: encodeBase64URL(credential.response.attestationObject),
        transports: credential.response.getTransports?.() || [],
      },
    };
  }

  async function get(publicKey) {
    if (!global.PublicKeyCredential || !navigator.credentials) {
      throw new Error('Dieser Browser unterstützt keine Passkeys.');
    }
    const credential = await navigator.credentials.get({ publicKey: authenticationOptions(publicKey) });
    if (!credential) throw new Error('Die Passkey-Anmeldung wurde abgebrochen.');
    const response = {
      clientDataJSON: encodeBase64URL(credential.response.clientDataJSON),
      authenticatorData: encodeBase64URL(credential.response.authenticatorData),
      signature: encodeBase64URL(credential.response.signature),
    };
    if (credential.response.userHandle) response.userHandle = encodeBase64URL(credential.response.userHandle);
    return {
      id: credential.id,
      rawId: encodeBase64URL(credential.rawId),
      type: credential.type,
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      response,
    };
  }

  global.WebAuthnClient = Object.freeze({ create, get });
}(window));
