import { useEffect } from 'react';
import { useAppStore } from '../store/app';
import { applyTheme } from '../utils/applyTheme';

export function useTheme() {
  const theme = useAppStore((s) => s.theme);
  const setTheme = useAppStore((s) => s.setTheme);
  const toggleTheme = useAppStore((s) => s.toggleTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  return { theme, setTheme, toggleTheme };
}
