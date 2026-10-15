/** Leaves the app for a page the server renders, such as the OIDC Provider's own endpoints. */
export const navigation = {
  leave(path: string): void {
    window.location.assign(path)
  },
}
