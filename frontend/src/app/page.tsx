"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { getSession } from "@/lib/api";

export default function RootPage() {
  const router = useRouter();

  useEffect(() => {
    getSession()
      .then(() => router.replace("/admin"))
      .catch(() => router.replace("/login"));
  }, [router]);

  return null;
}