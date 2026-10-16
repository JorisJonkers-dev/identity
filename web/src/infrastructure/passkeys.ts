/**
 * The browser's passkey API, behind one seam: the server's options go in as JSON, the browser's
 * answer comes out as JSON, and components never touch navigator.credentials themselves.
 */
export const passkeys = {
  supported(): boolean {
    return typeof PublicKeyCredential !== 'undefined' && 'parseCreationOptionsFromJSON' in PublicKeyCredential
  },

  async create(options: string): Promise<string> {
    const { publicKey } = JSON.parse(options) as { publicKey: PublicKeyCredentialCreationOptionsJSON }
    const credential = await navigator.credentials.create({ publicKey: PublicKeyCredential.parseCreationOptionsFromJSON(publicKey) })
    return answer(credential)
  },

  async get(options: string): Promise<string> {
    const { publicKey } = JSON.parse(options) as { publicKey: PublicKeyCredentialRequestOptionsJSON }
    const credential = await navigator.credentials.get({ publicKey: PublicKeyCredential.parseRequestOptionsFromJSON(publicKey) })
    return answer(credential)
  },
}

function answer(credential: Credential | null): string {
  if (!(credential instanceof PublicKeyCredential)) throw new Error('no passkey')
  return JSON.stringify(credential.toJSON())
}
