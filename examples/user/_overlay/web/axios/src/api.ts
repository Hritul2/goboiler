import axios from "axios"
import { createClient, ApiError } from "./gen/ts/v1"

// Your own instance: interceptors (auth, retries, logging) keep working.
const http = axios.create({ baseURL: "https://api.example.com" })
http.interceptors.request.use((cfg) => {
  cfg.headers.set("Authorization", `Bearer ${localStorage.getItem("token") ?? ""}`)
  return cfg
})

export const client = createClient({ axios: http })
// or, without an instance of your own: createClient({ baseURL: "https://api.example.com" })

export async function signUp(email: string, name: string, password: string) {
  try {
    const user = await client.createUser({ email, name, password })
    return user.createdAt
  } catch (e) {
    if (e instanceof ApiError && e.status === 0) {
      return "network error, try again" // code "NETWORK_ERROR": no response at all
    }
    if (e instanceof ApiError && e.code === "CONFLICT") {
      return "that email is already registered"
    }
    throw e
  }
}
