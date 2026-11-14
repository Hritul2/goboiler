import { createClient, ApiError } from "./gen/ts/v1"

export const client = createClient({
  baseUrl: "https://api.example.com",
  headers: () => ({ Authorization: `Bearer ${localStorage.getItem("token") ?? ""}` }), // optional
})

export async function signUp(email: string, name: string, password: string) {
  try {
    const user = await client.createUser({ email, name, password }) // typed in, zod-parsed out
    return user.createdAt // a real Date
  } catch (e) {
    if (e instanceof ApiError && e.code === "CONFLICT") {
      return "that email is already registered" // e.status === 409
    }
    throw e
  }
}
