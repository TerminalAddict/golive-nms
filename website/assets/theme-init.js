(() => {
  const key = 'golive-marketing-theme';
  let theme = 'light';
  try {
    const saved = localStorage.getItem(key);
    theme = saved === 'light' || saved === 'dark'
      ? saved
      : 'light';
  } catch {
    theme = 'light';
  }
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
})();
