"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { login } from "@/lib/api";
import PasswordField from "@/components/PasswordField";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setLoading(true);
    try {
      await login(email, password);
      router.push("/admin");
    } catch {
      setError("Invalid email or password");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="min-h-screen bg-[#f2f0ec] flex items-center justify-center p-6">
      <div className="w-full max-w-2xl bg-white rounded-lg shadow-sm p-2 grid grid-cols-1 md:grid-cols-2 gap-2">
        <div className="p-6 flex flex-col justify-center">
          <div className="text-sm font-medium mb-8">mesh</div>
          <h1 className="text-xl font-serif mb-5">Sign in</h1>
          <form onSubmit={handleSubmit} className="space-y-3">
            <div>
              <label className="block text-xs font-medium mb-1" htmlFor="email">
                Email address
              </label>
              <input
                id="email"
                type="email"
                required
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="Enter your email"
                className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
              />
            </div>
            <PasswordField
              id="password"
              label="Password"
              autoComplete="current-password"
              placeholder="Enter your password"
              value={password}
              onChange={setPassword}
            />
            {error && <p className="text-xs text-red-600">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50"
            >
              {loading ? "Signing in..." : "Sign in"}
            </button>
          </form>
        </div>
        <div className="relative rounded-md overflow-hidden min-h-[260px]">
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_30%_30%,_#5b8def,_transparent_55%),radial-gradient(circle_at_70%_60%,_#f2c14e,_transparent_60%),radial-gradient(circle_at_50%_90%,_#e8b23d,_transparent_55%)] blur-2xl" />
        </div>
      </div>
    </main>
  );
}
