import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const clientDir = path.resolve(__dirname, '..');
const repoRoot = path.resolve(clientDir, '..');
const outputDir = path.join(repoRoot, 'output', 'playwright');
const browserCacheDir = process.env.PLAYWRIGHT_BROWSERS_PATH || path.join(repoRoot, '.cache', 'ms-playwright');
const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:5173';
const headless = process.env.PLAYWRIGHT_HEADLESS !== 'false';

// 管理员与技师账号密码必须由外部环境显式提供，避免脚本继续依赖仓库内默认口令。
const adminAccount = {
  phone: process.env.ADMIN_PHONE || '0912345678',
  password: process.env.ADMIN_PASSWORD || '',
};

const technicianAccount = {
  phone: process.env.TECH_PHONE || '0987654321',
  password: process.env.TECH_PASSWORD || '',
};

// ensureOutputDir 统一准备截图与 JSON 结果目录，避免执行结果散落到各个临时路径。
async function ensureOutputDir() {
  await fs.mkdir(outputDir, { recursive: true });
}

// assertAccountEnv 在脚本启动前检查登录口令是否显式传入，避免误把空密码提交给登录页。
function assertAccountEnv() {
  if (!adminAccount.password) {
    throw new Error('缺少 ADMIN_PASSWORD，无法执行管理员登录冒烟测试');
  }
  if (!technicianAccount.password) {
    throw new Error('缺少 TECH_PASSWORD，无法执行技师登录冒烟测试');
  }
}

// saveJSON 把结构化结果落到 output/playwright，便于后续审查和 CI 采集。
async function saveJSON(filename, data) {
  await fs.writeFile(path.join(outputDir, filename), `${JSON.stringify(data, null, 2)}\n`, 'utf8');
}

// loginFromLoginPage 强制从登录页进入，保证每条用户链路都真实覆盖输入框、登录按钮和鉴权接口。
async function loginFromLoginPage(page, account) {
  await page.goto(baseURL, { waitUntil: 'domcontentloaded' });
  await page.getByTestId('form-login').waitFor({ state: 'visible', timeout: 15000 });
  await page.getByTestId('input-phone').fill(account.phone);
  await page.getByTestId('input-password').fill(account.password);
  await page.getByTestId('button-login').click();
}

// waitForHeaderTitle 统一等待页面主标题切换完成，避免导航动画还没结束就开始断言。
async function waitForHeaderTitle(page, expectedText) {
  await page.getByTestId('text-header-title').waitFor({ state: 'visible', timeout: 15000 });
  await page.getByTestId('text-header-title').waitFor({ state: 'visible', timeout: 15000 });
  await page.waitForFunction(
    ([testId, expected]) => {
      const element = document.querySelector(`[data-testid="${testId}"]`);
      return element?.textContent?.includes(expected);
    },
    ['text-header-title', expectedText],
    { timeout: 15000 },
  );
}

// runAdminFlow 覆盖管理员登录、首页、任务清单、创建页和退出，验证管理端关键导航没有回归。
async function runAdminFlow(browser) {
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 1440, height: 960 },
  });
  const page = await context.newPage();

  try {
    await loginFromLoginPage(page, adminAccount);
    await page.getByTestId('text-dashboard-greeting').waitFor({ state: 'visible', timeout: 15000 });
    await waitForHeaderTitle(page, '首頁總覽');

    const dashboardGreeting = (await page.getByTestId('text-dashboard-greeting').textContent())?.trim();
    assert.ok(dashboardGreeting?.includes('管理員'), '管理员登录后未进入首页总览');

    await page.screenshot({
      path: path.join(outputDir, 'admin-dashboard.png'),
      fullPage: true,
    });

    await page.getByTestId('nav-list').click();
    await waitForHeaderTitle(page, '任務清單');
    await page.getByTestId('input-search').waitFor({ state: 'visible', timeout: 15000 });

    const listRowCount = await page.locator('[data-testid^="row-appointment-"]').count();

    await page.getByTestId('nav-create').click();
    await waitForHeaderTitle(page, '新增預約單');
    await page.getByTestId('form-create-appointment').waitFor({ state: 'visible', timeout: 15000 });

    await page.screenshot({
      path: path.join(outputDir, 'admin-create.png'),
      fullPage: true,
    });

    await page.getByTestId('button-logout').click();
    await page.getByTestId('button-logout-confirm').click();
    await page.getByTestId('form-login').waitFor({ state: 'visible', timeout: 15000 });

    return {
      flow: 'admin',
      status: 'passed',
      dashboardGreeting,
      listRowCount,
      screenshots: ['admin-dashboard.png', 'admin-create.png'],
    };
  } finally {
    await context.close();
  }
}

// runTechnicianFlow 覆盖技师登录、今日任务、个人页和退出，验证技师端核心工作台可用。
async function runTechnicianFlow(browser) {
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 430, height: 932 },
  });
  const page = await context.newPage();

  try {
    await loginFromLoginPage(page, technicianAccount);
    await page.getByTestId('text-greeting').waitFor({ state: 'visible', timeout: 15000 });
    await page.getByTestId('tech-bottom-nav').waitFor({ state: 'visible', timeout: 15000 });

    const todayCountText = (await page.getByTestId('text-today-count').textContent())?.trim();
    assert.ok(todayCountText?.includes('今日任務'), '技师登录后未进入今日任务页');

    await page.screenshot({
      path: path.join(outputDir, 'technician-today.png'),
      fullPage: true,
    });

    await page.getByTestId('tab-profile').click();
    await page.getByTestId('text-profile-name').waitFor({ state: 'visible', timeout: 15000 });
    const profileName = (await page.getByTestId('text-profile-name').textContent())?.trim();
    assert.ok(profileName, '技师个人页未显示姓名');

    await page.screenshot({
      path: path.join(outputDir, 'technician-profile.png'),
      fullPage: true,
    });

    await page.getByTestId('button-logout').click();
    await page.getByTestId('button-logout-confirm').click();
    await page.getByTestId('form-login').waitFor({ state: 'visible', timeout: 15000 });

    return {
      flow: 'technician',
      status: 'passed',
      todayCountText,
      profileName,
      screenshots: ['technician-today.png', 'technician-profile.png'],
    };
  } finally {
    await context.close();
  }
}

async function main() {
  await ensureOutputDir();
  assertAccountEnv();

  const runMeta = {
    baseURL,
    browserCacheDir,
    headless,
    startedAt: new Date().toISOString(),
  };

  let browser;
  const results = [];

  try {
    // browser launch 固定走仓库内 Playwright 浏览器缓存，避免再次回退到系统 Chrome 通道。
    browser = await chromium.launch({
      headless,
      executablePath: chromium.executablePath(),
    });

    results.push(await runAdminFlow(browser));
    results.push(await runTechnicianFlow(browser));

    const summary = {
      ...runMeta,
      finishedAt: new Date().toISOString(),
      status: 'passed',
      results,
    };

    await saveJSON('user-smoke-summary.json', summary);
    console.log(JSON.stringify(summary, null, 2));
  } catch (error) {
    const summary = {
      ...runMeta,
      finishedAt: new Date().toISOString(),
      status: 'failed',
      results,
      error: error instanceof Error ? error.message : String(error),
    };
    await saveJSON('user-smoke-summary.json', summary);
    throw error;
  } finally {
    await browser?.close();
  }
}

main().catch((error) => {
  console.error('[user-smoke] 执行失败:', error);
  process.exitCode = 1;
});
