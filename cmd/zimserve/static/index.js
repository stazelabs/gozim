(function(){
  var col = 0, asc = true;
  var ths = document.querySelectorAll('th[data-col]');
  var tbody = document.querySelector('tbody');
  function sort(c, a) {
    col = c; asc = a;
    ths.forEach(function(th, i) {
      var arrow = th.querySelector('.arrow');
      arrow.textContent = i === c ? (a ? ' \u25b2' : ' \u25bc') : '';
      th.classList.toggle('sorted', i === c);
    });
    var rows = Array.from(tbody.rows);
    rows.sort(function(ra, rb) {
      var av = ra.cells[c].dataset.val;
      var bv = rb.cells[c].dataset.val;
      var cmp = c === 3 ? +av - +bv : av.toLowerCase().localeCompare(bv.toLowerCase());
      return a ? cmp : -cmp;
    });
    rows.forEach(function(r){ tbody.appendChild(r); });
  }
  ths.forEach(function(th, i){
    th.addEventListener('click', function(){ sort(i, col === i ? !asc : true); });
  });
  sort(0, true);

  var input = document.getElementById('search-input');
  var zimSelect = document.getElementById('search-zim');
  var resultsDiv = document.getElementById('search-results');
  var timer = null;
  var activeReq = 0;
  function showResults() { resultsDiv.classList.add('active'); }
  function hideResults() { resultsDiv.classList.remove('active'); }
  input.addEventListener('input', function(){
    clearTimeout(timer);
    var q = input.value.trim();
    if (!q) { resultsDiv.innerHTML = ''; hideResults(); return; }
    timer = setTimeout(function(){
      var slug = zimSelect.value;
      var url = slug === '_all'
        ? '/_search?q=' + encodeURIComponent(q)
        : '/' + encodeURIComponent(slug) + '/_search?q=' + encodeURIComponent(q);
      var reqId = ++activeReq;
      resultsDiv.innerHTML = '<div class="loading">Searching</div>';
      showResults();
      fetch(url)
        .then(function(r){ return r.json(); })
        .then(function(data){
          if (reqId !== activeReq) return;
          if (!data.length) {
            resultsDiv.innerHTML = '<div class="empty">No results</div>';
            return;
          }
          resultsDiv.innerHTML = data.map(function(r){
            var el = document.createElement('a');
            el.href = r.path;
            el.textContent = r.title;
            return el.outerHTML;
          }).join('');
        })
        .catch(function(){
          if (reqId !== activeReq) return;
          resultsDiv.innerHTML = '<div class="empty">Search failed</div>';
        });
    }, 200);
  });
  input.addEventListener('focus', function(){
    if (resultsDiv.innerHTML) showResults();
  });
  document.addEventListener('mousedown', function(e){
    if (!e.target.closest('.search-wrap')) hideResults();
  });
  document.getElementById('random-btn').addEventListener('click', function(){
    var slug = zimSelect.value;
    var url = slug === '_all' ? '/_random' : '/' + encodeURIComponent(slug) + '/-/random';
    window.location.href = url;
  });
})();
