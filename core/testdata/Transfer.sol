pragma solidity ^0.4.1;

contract Transfer {
address owner;
int256 sum;
event transationEvent(address from, address to, uint256 amount);
event sumEvent(int256 old, int256 _sum, int256 add);
event balanceEvent(address sender, uint256 beforeBalance, uint256 afterBalance);

// TODO: use constructor
function Transfer() public payable {
    owner = msg.sender;
}


// // Notic, msg.sender has two transaction, one transfer to `to`, one transfer to `contract(because payable)`
// function sendPay(address to, uint256 amount) public payable {
//     uint256 beforeBalance = msg.sender.balance;
//     require(msg.sender.balance >= (amount + msg.value));
//     // transfer to `to`
//     to.transfer(amount);
//     transationEvent(msg.sender, to, amount);
//     uint256 afterBalance = msg.sender.balance;
//     balanceEvent(msg.sender, beforeBalance, afterBalance);
// }

// not use `send` function to transfer, because it just return false, does not throw an exception
// so, it use `send`, must
// caller --> contract --> to
function send(address to, uint256 amount) public payable{
    require(msg.value >= amount);
    to.transfer(amount);
    transationEvent(msg.sender, to, amount);
}


function sends(address []to, uint256 amount) public payable{
    uint256 total = amount * to.length;
    require(msg.value >= total);
    for (uint256 i =0; i < to.length; i ++) {
        address receiver = to[i];
        receiver.transfer(amount);
        transationEvent(msg.sender, receiver, amount);
    }
}

// send the contract money to caller
// Notic: payable
function contractTransferout(uint256 amount) public payable {
    // require(balance >= amount);
    msg.sender.transfer(amount);
    // balance -= amount;
    // Notic, owner is not contract address
    transationEvent(owner, msg.sender, amount);
}

function add(int256 a) public {
    int256 old = sum;
    sum += a;
    sumEvent(old, sum, a);
}

function sub(int256 a) public {
    int256 old = sum;
    sum -= a;
    sumEvent(old, sum, a);
}

function sumResult() public view returns (int256) {
    return sum;
}

function ownerAddress() public view returns (address) {
    return owner;
}

function accountBalance(address account) public view returns (uint256) {
    return account.balance;
}
}