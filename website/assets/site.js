const navToggle = document.querySelector('.nav-toggle');
const navLinks = document.querySelector('.nav-links');
const themeToggle = document.querySelector('.theme-toggle');
const themeLabel = themeToggle?.querySelector('.theme-label');
const themeKey = 'golive-marketing-theme';

const showTheme = (theme) => {
  const next = theme === 'dark' ? 'light' : 'dark';
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
  if (themeToggle) {
    themeToggle.setAttribute('aria-label', `Switch to ${next} mode`);
    themeToggle.setAttribute('title', `Switch to ${next} mode`);
    themeToggle.setAttribute('aria-pressed', String(theme === 'light'));
  }
  if (themeLabel) themeLabel.textContent = next === 'light' ? 'Light' : 'Dark';
};

showTheme(document.documentElement.dataset.theme || 'dark');

themeToggle?.addEventListener('click', () => {
  const theme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
  showTheme(theme);
  try {
    localStorage.setItem(themeKey, theme);
  } catch {
    // The selected theme still applies for this page if storage is unavailable.
  }
});

navToggle?.addEventListener('click', () => {
  const open = navLinks?.classList.toggle('open') ?? false;
  navToggle.setAttribute('aria-expanded', String(open));
});

navLinks?.querySelectorAll('a').forEach((link) => {
  link.addEventListener('click', () => {
    navLinks.classList.remove('open');
    navToggle?.setAttribute('aria-expanded', 'false');
  });
});

document.querySelectorAll('.code-block').forEach((block) => {
  const pre = block.querySelector('pre');
  if (!pre) return;
  const button = document.createElement('button');
  button.className = 'copy-button';
  button.type = 'button';
  button.textContent = 'Copy';
  button.setAttribute('aria-label', 'Copy command');
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(pre.innerText);
      button.textContent = 'Copied';
      button.classList.add('copied');
      window.setTimeout(() => {
        button.textContent = 'Copy';
        button.classList.remove('copied');
      }, 1600);
    } catch {
      button.textContent = 'Select text';
    }
  });
  block.append(button);
});

document.querySelectorAll('[data-tabs]').forEach((tabs) => {
  const buttons = [...tabs.querySelectorAll('[role="tab"]')];
  const panels = [...tabs.querySelectorAll('[role="tabpanel"]')];
  buttons.forEach((button) => {
    button.addEventListener('click', () => {
      buttons.forEach((item) => item.setAttribute('aria-selected', String(item === button)));
      panels.forEach((panel) => {
        panel.hidden = panel.id !== button.getAttribute('aria-controls');
      });
    });
  });
});

const sections = [...document.querySelectorAll('.docs-content section[id]')];
const docLinks = [...document.querySelectorAll('.docs-nav a[href^="#"]')];
if (sections.length && 'IntersectionObserver' in window) {
  const observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];
    if (!visible) return;
    docLinks.forEach((link) => link.classList.toggle('active', link.hash === `#${visible.target.id}`));
  }, { rootMargin: '-15% 0px -70% 0px', threshold: [0, .25, .6] });
  sections.forEach((section) => observer.observe(section));
}

document.querySelectorAll('[data-year]').forEach((element) => {
  element.textContent = String(new Date().getFullYear());
});
