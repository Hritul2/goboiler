import { useState } from "react"
import { createHooks } from "./gen/ts/v1"
import { client } from "./api" // the createClient(...) from the fetch or axios snippet above

// The same component works with either generated client: hooks.ts is identical.
export const { useCreateUser } = createHooks(client)

const fields = ["email", "name", "password"] as const

export function SignupForm() {
  const [values, setValues] = useState({ email: "", name: "", password: "" })
  // Server-side validation messages, keyed by JSON field name: { email: ["must be a valid email address"] }
  const [errors, setErrors] = useState<Record<string, string[]>>({})

  const createUser = useCreateUser({
    onSuccess: () => setErrors({}),
    onError: (e) => setErrors(e.fields ?? {}), // e is an ApiError; `fields` only exists on validation failures
  })

  return (
    <form
      onSubmit={(ev) => {
        ev.preventDefault()
        createUser.mutate(values)
      }}
    >
      {fields.map((field) => (
        <label key={field}>
          {field}
          <input
            name={field}
            type={field === "password" ? "password" : "text"}
            value={values[field]}
            onChange={(ev) => setValues({ ...values, [field]: ev.target.value })}
          />
          {errors[field]?.map((message) => (
            <p key={message} role="alert">
              {message}
            </p>
          ))}
        </label>
      ))}
      {/* Errors that are not about a field (CONFLICT, NETWORK_ERROR, 500, ...) */}
      {createUser.error && !createUser.error.fields && <p role="alert">{createUser.error.message}</p>}
      <button disabled={createUser.isPending}>Sign up</button>
    </form>
  )
}
