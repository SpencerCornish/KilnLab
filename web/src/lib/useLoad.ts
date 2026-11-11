import { useCallback, useEffect, useState, type DependencyList } from 'react'

/** Loads data on mount (and when deps change). setData lets mutations swap in fresh results. */
export function useLoad<T>(fn: () => Promise<T>, deps: DependencyList) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(fn, deps)
  const reload = useCallback(async () => {
    try {
      setData(await load())
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [load])

  useEffect(() => {
    let live = true
    load()
      .then((d) => live && (setData(d), setError(null)))
      .catch((e) => live && setError(e instanceof Error ? e.message : String(e)))
    return () => {
      live = false
    }
  }, [load])

  return { data, error, reload, setData }
}
