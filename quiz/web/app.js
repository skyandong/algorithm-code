'use strict';

// 答题页面的全部交互逻辑：选知识点、出题、提交批改、重练错题、查看历史。
// 不依赖任何框架与构建步骤，直接由后端托管。

const DRAW_COUNT = 5;
const OPTION_KEYS = ['A', 'B', 'C', 'D', 'E', 'F'];
const USER_KEY = 'quiz-user';
const SESSION_KEY = 'quiz-session';

const MODULE_NAMES = {
  golang: 'Go 语言',
  redis: 'Redis',
  mysql: 'MySQL',
  'design-pattern': '设计模式与架构',
  distributed: '分布式',
  'system-design': '系统设计',
  microservice: '微服务与可观测性',
  akafka: 'Kafka',
  elasticsearch: 'Elasticsearch',
  tls: 'TLS',
  nginx: 'Nginx',
  interview: '面试',
};

const state = {
  topics: [],
  quiz: null,       // { topic, topicName, mode, questions }
  answers: {},      // qid -> 选项下标
  activeIndex: 0,   // 键盘操作指向的题目序号
  submitted: false,
  loading: false,
  result: null,
  resultFilter: 'all',
  partialConfirmed: false,
};

const $ = (id) => document.getElementById(id);
const hasAnswer = (qid) => Object.prototype.hasOwnProperty.call(state.answers, qid);

/* ---------------- 用户标识 ---------------- */

function userId() {
  let id = localStorage.getItem(USER_KEY);
  if (!id) {
    id = 'u-' + Math.random().toString(36).slice(2, 10) + Date.now().toString(36);
    localStorage.setItem(USER_KEY, id);
  }
  return id;
}

/* ---------------- 请求封装 ---------------- */

async function api(path, options = {}) {
  const url = path + (path.includes('?') ? '&' : '?') + 'user=' + encodeURIComponent(userId());
  const resp = await fetch(url, options);
  let body = null;
  try {
    body = await resp.json();
  } catch (_) {
    throw new Error('服务端返回了非 JSON 内容（HTTP ' + resp.status + '）');
  }
  if (!resp.ok) {
    throw new Error(body && body.error ? body.error : 'HTTP ' + resp.status);
  }
  return body;
}

let toastTimer = null;
function toast(msg, warn = false) {
  const el = $('toast');
  el.textContent = msg;
  el.className = 'toast' + (warn ? ' warn' : '');
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 3600);
}

/* ---------------- 未提交作答的暂存 ---------------- */

// 一轮卷子没提交就刷新或离开，作答会随会话存下来，回来还能接着答。
function saveSession() {
  if (!state.quiz || state.submitted) return;
  try {
    localStorage.setItem(SESSION_KEY, JSON.stringify({ quiz: state.quiz, answers: state.answers }));
  } catch (_) {
    // 存储写不进去（隐私模式、配额满）只影响恢复能力，答题本身照常。
  }
}

function readSession() {
  try {
    const raw = localStorage.getItem(SESSION_KEY);
    if (!raw) return null;
    const saved = JSON.parse(raw);
    if (!saved || !saved.quiz || !Array.isArray(saved.quiz.questions) || !saved.quiz.questions.length) {
      return null;
    }
    return { quiz: saved.quiz, answers: saved.answers || {} };
  } catch (_) {
    return null;
  }
}

function clearSession() {
  try {
    localStorage.removeItem(SESSION_KEY);
  } catch (_) {
    // 删不掉也没有副作用，下次读到的仍是同一份，最多多问一次。
  }
}

// 知识点列表上的「继续上次未提交的作答」提示条。
function renderResumeBar() {
  const bar = $('resume-bar');
  const saved = readSession();
  if (!saved) {
    bar.hidden = true;
    return;
  }
  const answered = saved.quiz.questions.filter((q) => hasAnswerIn(saved.answers, q.qid)).length;
  bar.innerHTML = '';
  const text = document.createElement('span');
  text.textContent = `有一轮没提交的作答：${saved.quiz.topicName}（已答 ${answered} / ${saved.quiz.questions.length}）`;
  const resume = document.createElement('button');
  resume.type = 'button';
  resume.className = 'link';
  resume.textContent = '继续答';
  resume.addEventListener('click', () => restoreSession(saved));
  const drop = document.createElement('button');
  drop.type = 'button';
  drop.className = 'link quiet';
  drop.textContent = '丢弃';
  drop.addEventListener('click', () => {
    clearSession();
    renderResumeBar();
    toast('已丢弃未提交的作答');
  });
  bar.append(text, resume, drop);
  bar.hidden = false;
}

function hasAnswerIn(answers, qid) {
  return Object.prototype.hasOwnProperty.call(answers, qid);
}

/* ---------------- 知识点列表 ---------------- */

async function loadTopics() {
  const data = await api('/api/topics');
  state.topics = data.topics || [];

  const total = state.topics.reduce((sum, t) => sum + t.total, 0);
  const done = state.topics.reduce((sum, t) => sum + t.done, 0);
  const correct = state.topics.reduce((sum, t) => sum + t.correct, 0);
  const wrong = state.topics.reduce((sum, t) => sum + t.wrong, 0);
  $('bank-summary').textContent = done
    ? `已做 ${done} / ${total} 题 · 正确率 ${Math.round((correct / done) * 100)}% · 错题 ${wrong} 道`
    : `题库 ${total} 题 · 还没开始作答`;

  const box = $('topics');
  if (!state.topics.length) {
    box.innerHTML = '<p class="empty">题库还是空的。先启动服务完成题库导入。</p>';
    return;
  }

  const groups = new Map();
  for (const t of state.topics) {
    if (!groups.has(t.module)) groups.set(t.module, []);
    groups.get(t.module).push(t);
  }

  const html = [];
  for (const [module, list] of groups) {
    html.push('<div class="group">');
    html.push(`<div class="group-name">${escapeHTML(MODULE_NAMES[module] || module)}</div>`);
    html.push('<div class="topics">');
    for (const t of list) {
      const pct = t.total ? Math.round((t.done / t.total) * 100) : 0;
      const short = t.remaining < DRAW_COUNT;
      html.push(`
        <div class="topic" data-slug="${escapeHTML(t.slug)}" data-name="${escapeHTML(t.name)}">
          <button class="topic-main" type="button" data-mode="new" ${short ? 'disabled' : ''}>
            <span class="topic-name">${escapeHTML(t.name)}</span>
            <span class="topic-foot">
              <span class="bar"><i style="width:${pct}%"></i></span>
              <span>${t.done} / ${t.total}</span>
            </span>
            <span class="topic-foot">
              <span>${short ? `新题只剩 ${t.remaining} 道` : `出 ${DRAW_COUNT} 道新题`}</span>
              ${t.wrong ? `<span class="badge-wrong">错题 ${t.wrong}</span>` : ''}
            </span>
          </button>
          ${t.wrong ? `<button class="topic-wrong" type="button" data-mode="wrong">
              重练错题 ${t.wrong} 道
            </button>` : ''}
        </div>`);
    }
    html.push('</div></div>');
  }
  box.innerHTML = html.join('');
}

/* ---------------- 出题 ---------------- */

async function startQuiz(slug, name, mode) {
  if (state.loading) return;
  state.loading = true;
  const box = $('topics');
  box.classList.add('loading');

  try {
    const data = await api(`/api/quiz?topic=${encodeURIComponent(slug)}&mode=${mode}&n=${DRAW_COUNT}`);
    if (!data.available) {
      toast(data.message || '这个知识点暂时没有可出的题。', true);
      await loadTopics();
      return;
    }
    state.quiz = {
      topic: slug,
      topicName: data.topicName || name,
      mode: data.mode || mode,
      questions: data.questions,
    };
    state.answers = {};
    state.activeIndex = 0;
    state.submitted = false;
    state.partialConfirmed = false;
    renderQuiz();
    saveSession();
  } catch (err) {
    toast(err.message, true);
  } finally {
    state.loading = false;
    box.classList.remove('loading');
  }
}

function restoreSession(saved) {
  state.quiz = saved.quiz;
  state.answers = saved.answers;
  state.activeIndex = 0;
  state.submitted = false;
  state.partialConfirmed = false;
  renderQuiz();
  renderResumeBar();
}

function renderQuiz() {
  const quiz = state.quiz;
  $('topics-section').hidden = true;
  $('result-section').hidden = true;
  $('quiz-section').hidden = false;

  $('quiz-title').textContent = (quiz.mode === 'wrong' ? '错题重练 · ' : '答题 · ') + quiz.topicName;
  $('submit-btn').disabled = false;
  $('submit-btn').textContent = '提交答卷';
  $('quiz-warning').hidden = true;
  $('kbd-hint').textContent = '快捷键：1-4 或 A-D 选选项 · ↑↓ 或 J K 切题 · Enter 提交 · Esc 返回';

  $('questions').innerHTML = quiz.questions.map(renderQuestion).join('');
  for (const input of $('questions').querySelectorAll('input[type=radio]')) {
    const picked = state.answers[input.dataset.qid];
    input.checked = picked !== undefined && picked === Number(input.value);
    input.closest('.opt').classList.toggle('picked', input.checked);
  }

  paintActive();
  updateProgress();
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function renderQuestion(q, index) {
  const opts = q.options.map((text, i) => `
    <label class="opt">
      <input type="radio" name="q-${index}" data-qid="${escapeHTML(q.qid)}" value="${i}">
      <span><span class="opt-key">${OPTION_KEYS[i]}.</span> ${escapeHTML(text)}</span>
    </label>`).join('');

  return `
    <div class="q" data-qid="${escapeHTML(q.qid)}">
      <div class="q-head">
        <span class="q-index">第 ${index + 1} 题</span>
        <span class="diff" title="难度 ${q.difficulty} 级">${difficultyDots(q.difficulty)}</span>
        <span class="q-flag" hidden>未作答</span>
      </div>
      <div class="q-stem">${escapeHTML(q.stem)}</div>
      ${opts}
    </div>`;
}

function difficultyDots(level) {
  const n = Math.min(3, Math.max(1, level || 2));
  return '●'.repeat(n) + '○'.repeat(3 - n);
}

function updateProgress() {
  if (!state.quiz) return;
  let answered = 0;
  for (const node of $('questions').querySelectorAll('.q')) {
    const done = hasAnswer(node.dataset.qid);
    if (done) answered++;
    node.classList.toggle('unanswered', !done);
    const flag = node.querySelector('.q-flag');
    if (flag) flag.hidden = done;
  }

  const total = state.quiz.questions.length;
  const left = total - answered;
  $('quiz-progress').textContent = left
    ? `已作答 ${answered} / ${total} · 还差 ${left} 道`
    : `已作答 ${answered} / ${total} · 可以提交了`;

  if (left === 0) {
    state.partialConfirmed = false;
    $('quiz-warning').hidden = true;
    if (!state.submitted) $('submit-btn').textContent = '提交答卷';
  }
}

/* ---------------- 键盘操作 ---------------- */

function focusQuestion(target) {
  const nodes = [...$('questions').querySelectorAll('.q')];
  if (!nodes.length) return;
  let index = typeof target === 'number'
    ? target
    : nodes.findIndex((n) => n.dataset.qid === target);
  if (index < 0) index = 0;
  state.activeIndex = Math.min(nodes.length - 1, Math.max(0, index));
  paintActive();
  nodes[state.activeIndex].scrollIntoView({ block: 'center', behavior: 'smooth' });
}

function paintActive() {
  [...$('questions').querySelectorAll('.q')].forEach((node, i) => {
    node.classList.toggle('active', i === state.activeIndex);
  });
}

function choose(qid, index) {
  state.answers[qid] = index;
  for (const input of $('questions').querySelectorAll('input[type=radio]')) {
    if (input.dataset.qid !== qid) continue;
    input.checked = Number(input.value) === index;
    input.closest('.opt').classList.toggle('picked', input.checked);
  }
  updateProgress();
  saveSession();
}

document.addEventListener('keydown', (e) => {
  const tag = e.target && e.target.tagName;
  const typing = tag === 'TEXTAREA' || tag === 'SELECT' ||
    (tag === 'INPUT' && !/^(radio|checkbox)$/.test(e.target.type));
  if (typing) return;

  if (e.key === 'Escape') {
    if (!$('quiz-section').hidden) backToTopics();
    return;
  }
  if ($('quiz-section').hidden || !state.quiz || state.submitted) return;

  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault();
    submitQuiz();
    return;
  }
  if (e.metaKey || e.ctrlKey || e.altKey) return;

  const current = state.quiz.questions[state.activeIndex];
  if (!current) return;

  const digit = /^[1-9]$/.test(e.key) ? Number(e.key) : 0;
  const letter = /^[a-fA-F]$/.test(e.key) ? e.key.toUpperCase().charCodeAt(0) - 64 : 0;
  const pick = digit || letter;
  if (pick && pick <= current.options.length) {
    e.preventDefault();
    choose(current.qid, pick - 1);
    return;
  }

  if (e.key === 'ArrowDown' || e.key === 'j' || e.key === 'J') {
    e.preventDefault();
    focusQuestion(state.activeIndex + 1);
  } else if (e.key === 'ArrowUp' || e.key === 'k' || e.key === 'K') {
    e.preventDefault();
    focusQuestion(state.activeIndex - 1);
  } else if (e.key === 'Enter') {
    e.preventDefault();
    submitQuiz();
  }
});

/* ---------------- 提交与批改 ---------------- */

function showWarning(text, onJump) {
  const el = $('quiz-warning');
  el.innerHTML = '';
  const span = document.createElement('span');
  span.textContent = text;
  el.appendChild(span);
  if (onJump) {
    const jump = document.createElement('button');
    jump.type = 'button';
    jump.className = 'link';
    jump.textContent = '跳到第一道未答题';
    jump.addEventListener('click', onJump);
    el.appendChild(jump);
  }
  el.hidden = false;
}

async function submitQuiz() {
  if (!state.quiz || state.submitted) return;

  const missing = state.quiz.questions.filter((q) => !hasAnswer(q.qid));
  if (missing.length && !state.partialConfirmed) {
    // 第一次点提交只是提醒，再点一次才真的按现状交卷。
    state.partialConfirmed = true;
    $('submit-btn').textContent = `确认提交（${missing.length} 道没答）`;
    showWarning(
      `还有 ${missing.length} 道没作答，提交后会记为答错。再点一次提交就按现状批改。`,
      () => focusQuestion(missing[0].qid),
    );
    focusQuestion(missing[0].qid);
    return;
  }

  $('submit-btn').disabled = true;
  $('submit-btn').textContent = '正在批改…';

  const items = state.quiz.questions.map((q) => ({
    qid: q.qid,
    choice: hasAnswer(q.qid) ? state.answers[q.qid] : -1,
  }));

  let result;
  try {
    result = await api('/api/submit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId: userId(), topic: state.quiz.topic, items }),
    });
  } catch (err) {
    $('submit-btn').disabled = false;
    $('submit-btn').textContent = '提交答卷';
    toast(err.message, true);
    return;
  }

  state.submitted = true;
  state.result = result;
  state.resultFilter = 'all';
  clearSession();
  renderResult(result);
  try {
    await loadTopics();
    await loadHistory();
  } catch (_) {
    // 统计刷新失败不影响已经拿到的批改结果。
  }
  renderWrongButton();
}

function renderResult(result) {
  const quiz = state.quiz;
  $('quiz-section').hidden = true;
  $('result-section').hidden = false;

  const rate = result.total ? Math.round((result.correct / result.total) * 100) : 0;
  $('result-title').textContent = (quiz.mode === 'wrong' ? '错题重练结果 · ' : '批改结果 · ') + quiz.topicName;
  $('result-score').textContent = `${result.correct} / ${result.total} 正确（${rate}%）`;

  renderCompare(result);

  const wrongCount = result.items.filter((it) => !it.correct).length;
  const tabs = [
    ['all', `全部 ${result.total}`, false],
    ['wrong', `只看错题 ${wrongCount}`, wrongCount === 0],
  ];
  $('result-filters').innerHTML = tabs.map(([key, label, disabled]) => `
    <button type="button" class="tab${state.resultFilter === key ? ' on' : ''}"
            data-filter="${key}" ${disabled ? 'disabled' : ''}>${label}</button>`).join('');

  renderResultItems();
  renderWrongButton();
  $('again-btn').onclick = () => startQuiz(quiz.topic, quiz.topicName, 'new');
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function renderCompare(result) {
  const el = $('result-compare');
  const prev = result.previous;
  if (!prev) {
    el.className = 'notice';
    el.textContent = '这是这个知识点的第一次作答，还没有可对比的历史成绩。';
    el.hidden = false;
    return;
  }

  const prevRate = prev.total ? prev.correct / prev.total : 0;
  const rate = result.total ? result.correct / result.total : 0;
  const delta = Math.round((rate - prevRate) * 100);
  let verdict = '正确率与上次持平。';
  if (delta > 0) verdict = `正确率比上次高 ${delta} 个百分点。`;
  if (delta < 0) verdict = `正确率比上次低 ${-delta} 个百分点。`;

  el.className = 'notice ' + (delta > 0 ? 'ok' : delta < 0 ? 'bad' : '');
  el.textContent = `上次 ${prev.correct} / ${prev.total}，这次 ${result.correct} / ${result.total}，${verdict}`;
  el.hidden = false;
}

function renderWrongButton() {
  const topic = state.topics.find((t) => state.quiz && t.slug === state.quiz.topic);
  const count = topic ? topic.wrong : 0;
  const btn = $('wrong-btn');
  btn.hidden = count === 0;
  btn.textContent = `重练错题 ${count} 道`;
  btn.onclick = () => startQuiz(state.quiz.topic, state.quiz.topicName, 'wrong');
}

function renderResultItems() {
  const items = state.result ? state.result.items : [];
  const shown = state.resultFilter === 'wrong' ? items.filter((it) => !it.correct) : items;
  if (!shown.length) {
    $('results').innerHTML = '<p class="empty">这一轮没有错题，全部答对。</p>';
    return;
  }
  $('results').innerHTML = shown.map((item) => renderGraded(item, items.indexOf(item))).join('');
}

function renderGraded(item, index) {
  let verdict;
  if (!item.answered) {
    verdict = `未作答，正确答案是 ${OPTION_KEYS[item.answer]}`;
  } else if (item.correct) {
    verdict = `答对了，你选的是 ${OPTION_KEYS[item.chosen]}`;
  } else {
    verdict = `答错了，你选的是 ${OPTION_KEYS[item.chosen]}，正确答案是 ${OPTION_KEYS[item.answer]}`;
  }

  const opts = item.options.map((text, i) => {
    const classes = ['opt'];
    if (i === item.answer) classes.push('right');
    if (item.answered && i === item.chosen && !item.correct) classes.push('wrong');
    if (!item.answered && i === item.answer) classes.push('missed');
    const mark = i === item.answer ? ' ✓ 正确项' : (item.answered && i === item.chosen ? ' ✗ 你的选择' : '');
    return `<div class="${classes.join(' ')}">
        <span class="opt-key">${OPTION_KEYS[i]}.</span>
        <span>${escapeHTML(text)}${mark ? `<b>${mark}</b>` : ''}</span>
      </div>`;
  }).join('');

  return `
    <div class="q">
      <div class="q-head">
        <span class="q-index">第 ${index + 1} 题</span>
      </div>
      <div class="q-stem">${escapeHTML(item.stem)}</div>
      <div class="verdict ${item.correct ? 'ok' : 'bad'}">${verdict}</div>
      ${opts}
      <div class="explain">
        <span class="label">解析</span>
        ${escapeHTML(item.explanation)}
        ${item.source ? `<span class="source">出处：${escapeHTML(item.source)}</span>` : ''}
      </div>
    </div>`;
}

/* ---------------- 历史与重置 ---------------- */

async function loadHistory() {
  let data;
  try {
    data = await api('/api/history');
  } catch (_) {
    return;
  }
  const records = data.records || [];
  if (!records.length) {
    $('history').innerHTML = '<p class="empty">还没有作答记录。</p>';
    return;
  }
  $('history').innerHTML = records.map((r) => {
    const rate = r.total ? Math.round((r.correct / r.total) * 100) : 0;
    const cls = rate >= 80 ? 'good' : rate >= 60 ? 'mid' : 'poor';
    return `
      <div class="hrow">
        <span>${escapeHTML(r.topicName || r.topicSlug)}</span>
        <span class="score">${escapeHTML(r.finishedAt)} · <b class="${cls}">${r.correct}/${r.total}</b></span>
      </div>`;
  }).join('');
}

async function resetProgress() {
  if (!confirm('清空作答进度后，所有题目都会重新变成「没做过」，错题池也会清空。确定吗？')) return;
  try {
    await api('/api/reset', { method: 'POST' });
  } catch (err) {
    toast(err.message, true);
    return;
  }
  clearSession();
  toast('进度已清空');
  await loadTopics();
  await loadHistory();
  renderResumeBar();
}

/* ---------------- 视图切换与工具 ---------------- */

function backToTopics() {
  // 一轮没提交就返回，作答留在会话里，列表页会给出继续入口。
  if (state.quiz && !state.submitted) saveSession();

  $('quiz-section').hidden = true;
  $('result-section').hidden = true;
  $('topics-section').hidden = false;
  state.quiz = null;
  state.submitted = false;
  state.partialConfirmed = false;
  renderResumeBar();
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function escapeHTML(text) {
  return String(text == null ? '' : text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/* ---------------- 事件绑定与启动 ---------------- */

$('topics').addEventListener('click', (e) => {
  const btn = e.target.closest('button[data-mode]');
  if (!btn || btn.disabled) return;
  const card = btn.closest('.topic');
  startQuiz(card.dataset.slug, card.dataset.name, btn.dataset.mode);
});

$('questions').addEventListener('change', (e) => {
  const input = e.target;
  if (input.type !== 'radio') return;
  choose(input.dataset.qid, Number(input.value));
});

$('result-filters').addEventListener('click', (e) => {
  const tab = e.target.closest('button[data-filter]');
  if (!tab || tab.disabled) return;
  state.resultFilter = tab.dataset.filter;
  for (const btn of $('result-filters').querySelectorAll('button')) {
    btn.classList.toggle('on', btn === tab);
  }
  renderResultItems();
});

$('submit-btn').addEventListener('click', submitQuiz);
$('back-btn').addEventListener('click', backToTopics);
$('back-top-btn').addEventListener('click', backToTopics);
$('result-back-btn').addEventListener('click', backToTopics);
$('result-back-top-btn').addEventListener('click', backToTopics);
$('reset-btn').addEventListener('click', resetProgress);

(async function init() {
  const saved = readSession();
  try {
    await loadTopics();
    await loadHistory();
  } catch (err) {
    toast(err.message, true);
  }
  if (saved) {
    restoreSession(saved);
    toast('已恢复上次未提交的作答');
  } else {
    renderResumeBar();
  }
})();
